# Security, Privacy, And Legal Notes

This page is not legal advice. It is a practical checklist for operators. If you
run cameras in a workplace, public-facing location, apartment building, school,
shop, or commercial context, consult qualified legal counsel in your jurisdiction.

## Project License

DT Camera is source-available for personal and small non-commercial use.

Commercial use is forbidden without written permission from Duy Nguyen
(duyltv). See [LICENSE](../LICENSE).

## Privacy Responsibilities

Camera systems can capture personal data and sensitive activity.

Before operating DT Camera, consider:

- whether cameras are legally allowed in each location
- whether people need notice or consent
- whether audio recording is allowed
- how long recordings should be retained
- who can view live and playback footage
- how access is logged and reviewed
- how recordings are exported, shared, or deleted

## Vietnam Legal Awareness

For deployments in Vietnam or involving Vietnamese users, operators should be
aware of Vietnamese cybersecurity and personal-data rules, including:

- Vietnam Law on Cybersecurity No. 24/2018/QH14, effective from January 1, 2019
- Decree No. 53/2022/ND-CP, guiding implementation of parts of the Law on
  Cybersecurity
- Decree No. 13/2023/ND-CP on personal data protection

These rules may be relevant to systems that collect, store, process, transmit,
or provide access to personal data, camera footage, user accounts, logs, or
network services in Vietnam.

DT Camera does not guarantee legal compliance by itself. The operator remains
responsible for lawful configuration, deployment, notices, data retention,
security controls, and incident handling.

## Security Checklist

For production:

- Use HTTPS and set `COOKIE_SECURE=true`.
- Use strong admin and PostgreSQL passwords.
- Do not expose PostgreSQL to the internet.
- Do not expose direct backend/frontend ports unless debugging.
- Restrict firewall access to required ports only.
- Give normal users the minimum camera permissions they need.
- Review event logs and alerts.
- Keep Docker images and host OS patched.
- Back up PostgreSQL metadata and recording folders.
- Test restore procedures before relying on backups.

## Camera Placement Checklist

Avoid or carefully review cameras that capture:

- bedrooms, bathrooms, changing rooms, or private areas
- neighboring property
- public spaces beyond your legal authority
- audio where audio recording requires consent
- employee-only areas without proper notice or policy

## Data Retention

Use retention settings to delete old footage automatically.

Recommended defaults depend on the use case:

- home: keep only what is useful for incident review
- small office: define a written retention period
- sensitive sites: consult legal/security requirements

Shorter retention usually reduces privacy and breach risk.

## Incident Response

If footage or credentials may be exposed:

1. Disable affected accounts.
2. Rotate passwords and camera credentials.
3. Preserve relevant logs.
4. Check exported or downloaded recordings.
5. Review legal notification obligations.
6. Patch the system before restoring access.

## Legal Sources To Review

Because laws change, use official government or qualified legal sources before
deployment. Search for the latest text of:

- `Luật An ninh mạng 24/2018/QH14`
- `Nghị định 53/2022/NĐ-CP`
- `Nghị định 13/2023/NĐ-CP bảo vệ dữ liệu cá nhân`

Do not rely only on this documentation for legal compliance decisions.
