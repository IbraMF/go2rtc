package onvif

import (
	"bytes"
	"encoding/xml"
	"errors"
	"strconv"
)

// PTZProfile selects the same profile as the stream's subtype, and discovers
// zoom support once, rather than issuing GetProfiles for every movement.
func (c *Client) PTZProfile() (string, error) {
	b, err := c.MediaRequest(MediaGetProfiles)
	if err != nil {
		return "", err
	}
	var envelope struct {
		Body struct {
			Response struct {
				Profiles []struct {
					Token string `xml:"token,attr"`
					PTZ   *struct {
						ZoomSpace  string    `xml:"DefaultContinuousZoomVelocitySpace"`
						ZoomLimits *struct{} `xml:"ZoomLimits"`
					} `xml:"PTZConfiguration"`
				} `xml:"Profiles"`
			} `xml:"GetProfilesResponse"`
		} `xml:"Body"`
	}
	if err = xml.Unmarshal(b, &envelope); err != nil {
		return "", err
	}
	profiles := envelope.Body.Response.Profiles
	subtype := c.url.Query().Get("subtype")
	index := 0
	if subtype != "" {
		index, err = strconv.Atoi(subtype)
	}
	for i, profile := range profiles {
		if (err == nil && i == index) || (err != nil && profile.Token == subtype) {
			if profile.PTZ == nil {
				return "", errors.New("onvif: profile has no PTZ configuration")
			}
			c.hasZoom = profile.PTZ.ZoomSpace != "" || profile.PTZ.ZoomLimits != nil
			return profile.Token, nil
		}
	}
	return "", errors.New("onvif: wrong PTZ subtype")
}

const (
	PTZContinuousMove = "ContinuousMove"
	PTZStop           = "Stop"
	PTZGetPresets     = "GetPresets"
	PTZGotoPreset     = "GotoPreset"
)

// PTZRequest sends one of the supported PTZ operations to the camera.
func (c *Client) PTZRequest(operation string, params ...string) ([]byte, error) {
	var body string
	switch operation {
	case PTZContinuousMove:
		if len(params) < 4 {
			return nil, errors.New("onvif: missing PTZ velocity")
		}
		if !c.hasZoom && params[3] != "0.00" && params[3] != "0" {
			return nil, errors.New("onvif: profile does not support zoom")
		}

		// Omit zoom for profiles that do not advertise it. Some cameras reject
		// the entire command when an unsupported Zoom element is present.
		velocity := `<tt:PanTilt x="` + params[1] + `" y="` + params[2] + `"/>`
		if c.hasZoom {
			velocity += `<tt:Zoom x="` + params[3] + `"/>`
		}

		body = `<tptz:ContinuousMove xmlns:tptz="http://www.onvif.org/ver20/ptz/wsdl">
			<tptz:ProfileToken>` + escapePTZToken(params[0]) + `</tptz:ProfileToken>
			<tptz:Velocity>
				` + velocity + `
			</tptz:Velocity>
			<tptz:Timeout>PT1S</tptz:Timeout>
		</tptz:ContinuousMove>`
	case PTZGetPresets:
		if len(params) < 1 {
			return nil, errors.New("onvif: missing PTZ profile")
		}
		body = `<tptz:GetPresets xmlns:tptz="http://www.onvif.org/ver20/ptz/wsdl">
			<tptz:ProfileToken>` + escapePTZToken(params[0]) + `</tptz:ProfileToken>
		</tptz:GetPresets>`
	case PTZGotoPreset:
		if len(params) < 2 {
			return nil, errors.New("onvif: missing PTZ preset")
		}
		body = `<tptz:GotoPreset xmlns:tptz="http://www.onvif.org/ver20/ptz/wsdl">
			<tptz:ProfileToken>` + escapePTZToken(params[0]) + `</tptz:ProfileToken>
			<tptz:PresetToken>` + escapePTZToken(params[1]) + `</tptz:PresetToken>
		</tptz:GotoPreset>`
	case PTZStop:
		if len(params) < 1 {
			return nil, errors.New("onvif: missing PTZ profile")
		}
		body = `<tptz:Stop xmlns:tptz="http://www.onvif.org/ver20/ptz/wsdl">
			<tptz:ProfileToken>` + escapePTZToken(params[0]) + `</tptz:ProfileToken>
		</tptz:Stop>`
	default:
		return nil, errors.New("onvif: unsupported PTZ operation")
	}

	return c.Request(c.ptzURL, body)
}

// PTZPreset is a camera-stored position as returned by ONVIF GetPresets.
type PTZPreset struct {
	Token string `json:"token"`
	Name  string `json:"name"`
}

// PTZPresets lists the positions for a PTZ-capable media profile.
func (c *Client) PTZPresets(profileToken string) ([]PTZPreset, error) {
	b, err := c.PTZRequest(PTZGetPresets, profileToken)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Body struct {
			Response struct {
				Presets []struct {
					Token string `xml:"token,attr"`
					Name  string `xml:"Name"`
				} `xml:"Preset"`
			} `xml:"GetPresetsResponse"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(b, &envelope); err != nil {
		return nil, err
	}
	presets := make([]PTZPreset, 0, len(envelope.Body.Response.Presets))
	for _, preset := range envelope.Body.Response.Presets {
		if preset.Token != "" {
			presets = append(presets, PTZPreset{Token: preset.Token, Name: preset.Name})
		}
	}
	return presets, nil
}

func escapePTZToken(token string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(token))
	return b.String()
}
