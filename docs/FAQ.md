# FAQ

## Is DT Camera open source?

DT Camera is source-available for transparency and personal/small
non-commercial use.

It is not MIT and not OSI-approved open source because commercial use is
restricted. See [LICENSE](../LICENSE).

## Can I use it at home?

Yes. Personal home camera use is allowed under the project license.

## Can I use it for a company, customer, paid service, or commercial product?

No, not without written permission from Duy Nguyen (duyltv).

## Why Docker Compose?

The system needs PostgreSQL, Go services, a React frontend, ffmpeg, mounted
recording folders, and shared internal networking. Docker Compose keeps that
repeatable.

## Are RTSP URLs exposed to the frontend?

No. RTSP URLs are stored in the database and used by backend/recorder stream
processes. Frontend responses should only receive safe fields and backend URLs.

## Why HLS instead of WebRTC?

HLS is simpler, stable, browser-friendly, and easy to proxy. WebRTC can be added
later for lower-latency viewing.

## How delayed is live view?

HLS has some natural delay. The backend includes a freshness guard controlled by
`HLS_MAX_LAG_SECONDS`; if the newest HLS segment is too old, the stream is
restarted.

## Does recording depend on the UI being open?

No. Recording runs in the recorder container in the background.

## Where are video files stored?

In the host folder configured by `RECORDINGS_HOST_PATH`, mounted into containers
at `RECORDINGS_CONTAINER_PATH`.

## Can I put it behind a domain?

Yes. See [Domain And HTTPS Setup](DOMAIN_SETUP.md).

## Does the project provide legal compliance?

No. DT Camera is software. You are responsible for camera placement, notices,
consent, data protection, retention, cybersecurity obligations, and local law.
See [Security, Privacy, And Legal Notes](SECURITY_PRIVACY_LEGAL.md).
