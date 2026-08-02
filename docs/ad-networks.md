# Ad Networks Configuration

## Overview

Ad networks allow administrators to configure external ad providers that serve ads dynamically across the JapaConnect platform.

## Supported Providers

| Provider    | Description | client_id format |
| ----------- | ----------- | ---------------- |
| `adsense`     | Google AdSense | `ca-pub-XXXXXXXXXXXXXXXX` |
| `ezoic`       | Ezoic AI ad optimization | Site ID or integration key |
| `media_net`   | Media.net contextual ads | Publisher ID |
| `custom`      | Custom HTML/JavaScript ad code | Any identifier |

## Slot Mapping

The `slot_mapping` JSONB field maps logical slot names to provider-specific slot IDs.

### Example (AdSense)

```json
{
  "banner_728x90": 1234567890,
  "sidebar_300x250": 9876543210,
  "in_article": 1122334455
}
```

### Example (Ezoic)

```json
{
  "banner_728x90": "ezoic-banner-top",
  "sidebar_300x250": "ezoic-sidebar",
  "in_article": "ezoic-inline"
}
```

### Common Slot Names

| Slot Name        | Typical Size   | Location |
| ---------------- | -------------- | -------- |
| `banner_728x90`    | 728×90 px      | Page header / between posts |
| `sidebar_300x250`  | 300×250 px     | Right sidebar |
| `in_article`       | Responsive     | Within article content |
| `sidebar_top`      | Variable       | Top of right sidebar |
| `sidebar_bottom`   | Variable       | Bottom of right sidebar |
| `feed_inline`      | Variable       | Between feed items |

## Priority Ordering

When multiple ad networks are configured for the same provider:
- Higher `priority` values are preferred (e.g., priority 10 > priority 0)
- Only `is_active = true` networks are considered
- The frontend picks the highest-priority active network

## Security

- The `api_key` field is NEVER exposed to the frontend
- The public endpoint `GET /api/v1/ads/networks/config` returns only safe fields
- Admin endpoints require `ad.network.manage` permission

## API Reference

### Admin Routes (require auth + `ad.network.manage`)

| Method | Path | Description |
| ------ | ---- | ----------- |
| GET | `/admin/ads/networks` | List all networks (paginated) |
| POST | `/admin/ads/networks` | Create a new network |
| GET | `/admin/ads/networks/{id}` | Get a single network |
| PUT | `/admin/ads/networks/{id}` | Update a network |
| DELETE | `/admin/ads/networks/{id}` | Delete a network |

### Public Routes (no auth)

| Method | Path | Description |
| ------ | ---- | ----------- |
| GET | `/ads/networks/config` | Get active networks (no api_key) |
