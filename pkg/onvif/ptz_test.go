package onvif

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPTZProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `<Envelope><Body><GetProfilesResponse>
			<Profiles token="fixed"><PTZConfiguration>
				<DefaultContinuousZoomVelocitySpace>zoom</DefaultContinuousZoomVelocitySpace>
			</PTZConfiguration></Profiles>
		</GetProfilesResponse></Body></Envelope>`)
	}))
	defer server.Close()

	client := &Client{
		url:        mustURL(t, "onvif://camera"),
		mediaURL:   server.URL,
		httpClient: server.Client(),
	}
	token, err := client.PTZProfile()
	require.NoError(t, err)
	require.Equal(t, "fixed", token)
	require.True(t, client.hasZoom)
}

func TestPTZRequestEscapesTokens(t *testing.T) {
	var requestBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		requestBody = string(body)
		_, _ = io.WriteString(w, `<Envelope/>`)
	}))
	defer server.Close()

	client := &Client{
		url:        mustURL(t, "onvif://camera"),
		ptzURL:     server.URL,
		httpClient: server.Client(),
	}
	_, err := client.PTZRequest(PTZGotoPreset, `profile<&`, `door<&`)
	require.NoError(t, err)
	require.Contains(t, requestBody, `<tptz:ProfileToken>profile&lt;&amp;</tptz:ProfileToken>`)
	require.Contains(t, requestBody, `<tptz:PresetToken>door&lt;&amp;</tptz:PresetToken>`)

	_, err = client.PTZRequest("Unknown")
	require.EqualError(t, err, "onvif: unsupported PTZ operation")
}

func TestPTZPresets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `<Envelope><Body><GetPresetsResponse>
			<Preset token="home"><Name>Home</Name></Preset>
			<Preset token=""><Name>Ignored</Name></Preset>
		</GetPresetsResponse></Body></Envelope>`)
	}))
	defer server.Close()

	client := &Client{
		url:        mustURL(t, "onvif://camera"),
		ptzURL:     server.URL,
		httpClient: server.Client(),
	}
	presets, err := client.PTZPresets("fixed")
	require.NoError(t, err)
	require.Equal(t, []PTZPreset{{Token: "home", Name: "Home"}}, presets)
}

func mustURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	return u
}
