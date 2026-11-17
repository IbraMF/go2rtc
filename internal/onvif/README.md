# ONVIF

## ONVIF Client

[`new in v1.5.0`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.5.0)

The source is not very useful if you already know RTSP and snapshot links for your camera. But it can be useful if you don't.

**WebUI > Add** webpage supports ONVIF autodiscovery. Your server must be on the same subnet as the camera. If you use Docker, you must use "network host".

```yaml
streams:
  dahua1: onvif://admin:password@192.168.1.123
  reolink1: onvif://admin:password@192.168.1.123:8000
  tapo1: onvif://admin:password@192.168.1.123:2020
```

## PTZ controls

Open `stream.html?src=dahua1&ptz=1` to show pan, tilt and zoom controls without
renaming the stream. Holding a direction or zoom button starts movement immediately;
releasing sends a stop. The red center button sends an explicit stop.
Button feedback is clipped to the circular controls.

Controls are enabled only by `ptz=1` or `ptz=true`; the stream name has no special
meaning. With multiple `src` parameters, one `ptz` value applies to all streams;
repeated values apply in the same order as `src`.

The stream must include an `onvif://` source. The configured `subtype` selects the
PTZ profile too. Runtime stream changes and lists of sources are supported.
Fragments such as `#media=video#backchannel=0` are options for the underlying
stream producer; they are independent of PTZ and continue to be forwarded to it.

The browser serializes commands and renews movement every 300 ms while held.
Each move requests the ONVIF `PT1S` timeout, and go2rtc also sends a stop after
1.5 seconds without a renewal, retrying failed stops. This bounds movement if a
release event or browser connection is lost, provided the camera honors ONVIF
timeouts or remains reachable. Cameras must accept a one-second movement timeout.

Clients and profiles are cached; after initialization each command uses one SOAP
request. PTZ does not change video buffering or codecs. For a WebRTC-only viewer,
use `stream.html?src=dahua1&ptz=1&mode=webrtc`; the normal player keeps its existing
fallback modes when `mode` is omitted.

Custom clients can POST JSON to `api/ptz` relative to the configured API base path:

```json
{"src":"dahua1","action":"move","pan":0.5,"tilt":0,"zoom":0}
```

Renew movement while held and send `{"src":"dahua1","action":"stop"}` on release.
Velocities are clamped to `[-1,1]`. Success returns HTTP 204; invalid commands
return 400 and camera failures return 502.

## ONVIF Server

A regular camera has a single video source (`GetVideoSources`) and two profiles (`GetProfiles`).

Go2rtc has one video source and one profile per stream.

## Tested clients

Go2rtc works as ONVIF server:

- Happytime onvif client (windows)
- Home Assistant ONVIF integration (linux)
- Onvier (android)
- ONVIF Device Manager (windows)

PS. Supports only TCP transport for RTSP protocol. UDP and HTTP transports - unsupported yet.

## Tested cameras

Go2rtc works as ONVIF client:

- Dahua IPC-K42
- OpenIPC
- Reolink RLC-520A
- TP-Link Tapo TC60
