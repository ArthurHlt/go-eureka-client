package eureka

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Errors introduced by handling requests
var (
	ErrRequestCancelled = errors.New("sending request is cancelled")
)

type RawRequest struct {
	method       string
	relativePath string
	body         []byte
	cancel       <-chan bool
}

type InstanceInfo struct {
	HostName       string            `json:"hostName"`
	App            string            `json:"app"`
	IpAddr         string            `json:"ipAddr"`
	VipAddress     string            `json:"vipAddress"`
	Status         string            `json:"status"`
	Port           uint              `json:"port"`
	SecurePort     uint              `json:"securePort"`
	DataCenterInfo *DataCenterInfo   `json:"dataCenterInfo"`
	LeaseInfo      *LeaseInfo        `json:"leaseInfo"`
	Metadata       map[string]string `json:"metadata"`
}

type DataCenterInfo struct {
	Name     string             `json:"name"`
	Metadata DataCenterMetadata `json:"metadata"`
}

type DataCenterMetadata struct {
	AmiLaunchIndex   string `json:"ami-launch-index"`
	LocalHostname    string `json:"local-hostname"`
	AvailabilityZone string `json:"availability-zone"`
	InstanceId       string `json:"instance-id"`
	PublicIpv4       string `json:"public-ipv4"`
	PublicHostname   string `json:"public-hostname"`
	AmiManifestPath  string `json:"ami-manifest-path"`
	LocalIpv4        string `json:"local-ipv4`
	Hostname         string `json:"hostname"`
	AmiId            string `json:"ami-id"`
	InstanceType     string `json:"instance-type"`
}

type LeaseInfo struct {
	EvictionDurationInSecs uint `json:"evictionDurationInSecs"`
}

func NewRawRequest(method, relativePath string, body []byte, cancel <-chan bool) *RawRequest {
	return &RawRequest{
		method:       method,
		relativePath: relativePath,
		body:         body,
		cancel:       cancel,
	}
}

func NewInstanceInfo(hostName, app, ip string, port, ttl uint) *InstanceInfo {
	dataCenterInfo := &DataCenterInfo{
		Name: "MyOwn",
	}
	leaseInfo := &LeaseInfo{
		EvictionDurationInSecs: ttl,
	}
	return &InstanceInfo{
		HostName:       hostName,
		App:            app,
		IpAddr:         ip,
		VipAddress:     hostName + ":" + string(port),
		Status:         UP,
		Port:           port,
		SecurePort:     port,
		DataCenterInfo: dataCenterInfo,
		LeaseInfo:      leaseInfo,
		Metadata:       nil,
	}
}

// getCancelable issues a cancelable GET request
func (c *Client) getCancelable(endpoint string,
	cancel <-chan bool) (*RawResponse, error) {
	logger.Debugf("get %s [%s]", endpoint, c.cluster.Leader)
	p := endpoint

	req := NewRawRequest("GET", p, nil, cancel)
	resp, err := c.SendRequest(req)

	if err != nil {
		return nil, err
	}

	return resp, nil
}

// get issues a GET request
func (c *Client) get(endpoint string) (*RawResponse, error) {
	return c.getCancelable(endpoint, nil)
}

// put issues a PUT request
func (c *Client) put(endpoint string, body []byte) (*RawResponse, error) {

	logger.Debugf("put %s, %s, [%s]", endpoint, body, c.cluster.Leader)
	p := endpoint

	req := NewRawRequest("PUT", p, body, nil)
	resp, err := c.SendRequest(req)

	if err != nil {
		return nil, err
	}

	return resp, nil
}

// post issues a POST request
func (c *Client) post(endpoint string, body []byte) (*RawResponse, error) {
	logger.Debugf("post %s, %s, [%s]", endpoint, body, c.cluster.Leader)
	p := endpoint

	req := NewRawRequest("POST", p, body, nil)
	resp, err := c.SendRequest(req)

	if err != nil {
		return nil, err
	}

	return resp, nil
}

// delete issues a DELETE request
func (c *Client) delete(endpoint string) (*RawResponse, error) {
	logger.Debugf("delete %s [%s]", endpoint, c.cluster.Leader)
	p := endpoint

	req := NewRawRequest("DELETE", p, nil, nil)
	resp, err := c.SendRequest(req)

	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *Client) SendRequest(rr *RawRequest) (*RawResponse, error) {

	var req *http.Request
	var resp *http.Response
	var httpPath string
	var err error
	var respBody []byte

	var numReqs = 1

	checkRetry := c.CheckRetry
	if checkRetry == nil {
		checkRetry = DefaultCheckRetry
	}

	cancelled := make(chan bool, 1)
	reqLock := new(sync.Mutex)

	if rr.cancel != nil {
		cancelRoutine := make(chan bool)
		defer close(cancelRoutine)

		go func() {
			select {
			case <-rr.cancel:
				cancelled <- true
				logger.Debug("send.request is cancelled")
			case <-cancelRoutine:
				return
			}

			// Repeat canceling request until this thread is stopped
			// because we have no idea about whether it succeeds.
			for {
				reqLock.Lock()
				c.httpClient.Transport.(*http.Transport).CancelRequest(req)
				reqLock.Unlock()

				select {
				case <-time.After(100 * time.Millisecond):
				case <-cancelRoutine:
					return
				}
			}
		}()
	}

	// If we connect to a follower and consistency is required, retry until
	// we connect to a leader
	sleep := 25 * time.Millisecond
	maxSleep := time.Second

	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			select {
			case <-cancelled:
				return nil, ErrRequestCancelled
			case <-time.After(sleep):
				sleep = sleep * 2
				if sleep > maxSleep {
					sleep = maxSleep
				}
			}
		}

		logger.Debug("Connecting to eureka: attempt ", attempt+1, " for ", rr.relativePath)

		httpPath = c.getHttpPath(false, rr.relativePath)

		logger.Debug("send.request.to ", httpPath, " | method ", rr.method)

		req, err := func() (*http.Request, error) {
			reqLock.Lock()
			defer reqLock.Unlock()

			if req, err = http.NewRequest(rr.method, httpPath, bytes.NewReader(rr.body)); err != nil {
				return nil, err
			}

			req.Header.Set("Content-Type",
				"application/json")
			return req, nil
		}()

		if err != nil {
			return nil, err
		}

		resp, err = c.httpClient.Do(req)
		defer func() {
			if resp != nil {
				resp.Body.Close()
			}
		}()

		// If the request was cancelled, return ErrRequestCancelled directly
		select {
		case <-cancelled:
			return nil, ErrRequestCancelled
		default:
		}

		numReqs++

		// network error, change a machine!
		if err != nil {
			logger.Debug("network error: ", err.Error())
			lastResp := http.Response{}
			if checkErr := checkRetry(c.cluster, numReqs, lastResp, err); checkErr != nil {
				return nil, checkErr
			}

			c.cluster.switchLeader(attempt % len(c.cluster.Machines))
			continue
		}

		// if there is no error, it should receive response
		logger.Debug("recv.response.from ", httpPath)

		if validHttpStatusCode[resp.StatusCode] {
			// try to read byte code and break the loop
			respBody, err = ioutil.ReadAll(resp.Body)
			if err == nil {
				logger.Debug("recv.success ", httpPath)
				break
			}
			// ReadAll error may be caused due to cancel request
			select {
			case <-cancelled:
				return nil, ErrRequestCancelled
			default:
			}

			if err == io.ErrUnexpectedEOF {
				// underlying connection was closed prematurely, probably by timeout
				// TODO: empty body or unexpectedEOF can cause http.Transport to get hosed;
				// this allows the client to detect that and take evasive action. Need
				// to revisit once code.google.com/p/go/issues/detail?id=8648 gets fixed.
				respBody = []byte{}
				break
			}
		}

		// if resp is TemporaryRedirect, set the new leader and retry
		if resp.StatusCode == http.StatusTemporaryRedirect {
			u, err := resp.Location()

			if err != nil {
				logger.Warning(err)
			} else {
				// Update cluster leader based on redirect location
				// because it should point to the leader address
				c.cluster.updateLeaderFromURL(u)
				logger.Debug("recv.response.relocate ", u.String())
			}
			resp.Body.Close()
			continue
		}

		if checkErr := checkRetry(c.cluster, numReqs, *resp,
			errors.New("Unexpected HTTP status code")); checkErr != nil {
			return nil, checkErr
		}
		resp.Body.Close()
	}

	r := &RawResponse{
		StatusCode: resp.StatusCode,
		Body:       respBody,
		Header:     resp.Header,
	}

	return r, nil
}

// DefaultCheckRetry defines the retrying behaviour for bad HTTP requests
// If we have retried 2 * machine number, stop retrying.
// If status code is InternalServerError, sleep for 200ms.
func DefaultCheckRetry(cluster *Cluster, numReqs int, lastResp http.Response,
	err error) error {

	if numReqs >= 2*len(cluster.Machines) {
		return newError(ErrCodeEurekaNotReachable,
			"Tried to connect to each peer twice and failed", 0)
	}

	code := lastResp.StatusCode
	if code == http.StatusInternalServerError {
		time.Sleep(time.Millisecond * 200)

	}

	logger.Warning("bad response status code", code)
	return nil
}

func (c *Client) getHttpPath(random bool, s ...string) string {
	var machine string
	if random {
		machine = c.cluster.Machines[rand.Intn(len(c.cluster.Machines))]
	} else {
		machine = c.cluster.Leader
	}

	fullPath := machine + "/eureka/" + version
	for _, seg := range s {
		fullPath = fullPath + "/" + seg
	}

	return fullPath
}

// buildValues builds a url.Values map according to the given value and ttl
func buildValues(value string, ttl uint64) url.Values {
	v := url.Values{}

	if value != "" {
		v.Set("value", value)
	}

	if ttl > 0 {
		v.Set("ttl", fmt.Sprintf("%v", ttl))
	}

	return v
}
