package onvif

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestForwardedRTSPPort(t *testing.T) {
	server := newStreamURIServer(t)
	defer server.Close()

	client := &Client{
		url:        mustURL(t, "onvif://viewer:secret@router.example:8000?subtype=fixed&rtsp_forwarded_port=8554"),
		mediaURL:   server.URL,
		httpClient: server.Client(),
	}
	rawURL, err := client.GetURI()
	require.NoError(t, err)
	require.Equal(t, "rtsp://viewer:secret@router.example:8554/live", rawURL)
}

func TestForwardedRTSPPortValidation(t *testing.T) {
	for _, port := range []string{"0", "65536", "not-a-port"} {
		t.Run(port, func(t *testing.T) {
			server := newStreamURIServer(t)
			defer server.Close()

			client := &Client{
				url:        mustURL(t, "onvif://router.example?subtype=fixed&rtsp_forwarded_port="+port),
				mediaURL:   server.URL,
				httpClient: server.Client(),
			}
			_, err := client.GetURI()
			require.EqualError(t, err, "onvif: invalid rtsp_forwarded_port")
		})
	}
}

func newStreamURIServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.True(t, strings.Contains(string(body), "GetStreamUri"))
		_, _ = io.WriteString(w, `<Envelope><Body><GetStreamUriResponse><MediaUri>
			<Uri>rtsp://10.0.0.2:554/live</Uri>
		</MediaUri></GetStreamUriResponse></Body></Envelope>`)
	}))
}
