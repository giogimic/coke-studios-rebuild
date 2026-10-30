package boot

import (
	"bytes"
	"gocoke/internal/client/net/amf"
	"io"
	"net/http"
	"testing"
)

type studioFixtureTransport struct{ payload []byte }

func (t studioFixtureTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(t.payload)), Header: make(http.Header)}, nil
}
func TestPrivateStudioLayoutIsNotSwapped(t *testing.T) {
	for _, layout := range []int{1, 2, 3, 4} {
		data, err := amf.EncodeRemotingResponse("/1/onResult", []any{map[string]any{"studioid": "owner1", "name": "Chosen studio", "ownername": "Owner", "layout": float64(layout)}})
		if err != nil {
			t.Fatal(err)
		}
		s := &Scene{httpClient: &http.Client{Transport: studioFixtureTransport{data}}, gatewayURL: "http://studio.test/sf/gateway"}
		studios, err := s.fetchPrivateStudios()
		if err != nil {
			t.Fatal(err)
		}
		if len(studios) != 1 || studios[0].Layout != layout {
			t.Errorf("server layout %d became %v", layout, studios)
		}
	}
}
