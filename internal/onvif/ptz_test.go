package onvif

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractFragment(t *testing.T) {
	base, fragment := extractFragment("onvif://camera?subtype=1#media=video#backchannel=0")
	require.Equal(t, "onvif://camera?subtype=1", base)
	require.Equal(t, "#media=video#backchannel=0", fragment)
}

func TestFormatSpeed(t *testing.T) {
	require.Equal(t, "-1.00", formatSpeed(-2))
	require.Equal(t, "0.25", formatSpeed(0.25))
	require.Equal(t, "1.00", formatSpeed(2))
}
