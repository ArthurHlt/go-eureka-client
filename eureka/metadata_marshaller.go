package eureka

import (
	"encoding/xml"
	"encoding/json"
	"regexp"
)
// StringMap is a map[string]string.
type MetaData struct {
	Map map[string]string
}


type Vraw struct {
	Content []byte `xml:",innerxml"`
}

func (s *MetaData) MarshalXML(e *xml.Encoder, start xml.StartElement) error {

	tokens := []xml.Token{start}

	for key, value := range s.Map {
		t := xml.StartElement{Name: xml.Name{"", key}}
		tokens = append(tokens, t, xml.CharData(value), xml.EndElement{t.Name})
	}

	tokens = append(tokens, xml.EndElement{start.Name})

	for _, t := range tokens {
		err := e.EncodeToken(t)
		if err != nil {
			return err
		}
	}

	// flush to ensure tokens are written
	err := e.Flush()
	if err != nil {
		return err
	}

	return nil
}

func (s *MetaData) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	s.Map = make(map[string]string)
	vraw := &Vraw{}
	d.DecodeElement(vraw, &start)
	dataInString := string(vraw.Content)
	regex, err := regexp.Compile("\\s*<([^<>]+)>([^<>]+)</[^<>]+>\\s*")
	if err != nil {
		return err
	}
	subMatches := regex.FindAllStringSubmatch(dataInString, -1)
	for _, subMatch := range subMatches {
		s.Map[subMatch[1]] = subMatch[2]
	}
	return nil
}

func (s *MetaData) MarshalJSON() ([]byte, error) {
	mapIt := make(map[string]string)
	for key, value := range s.Map {
		mapIt[key] = value
	}
	return json.Marshal(mapIt)
}
func (s *MetaData) UnmarshalJSON(data []byte) error {
	dataUnmarshal := make(map[string]string)
	err := json.Unmarshal(data, dataUnmarshal)
	s.Map = dataUnmarshal
	return err
}