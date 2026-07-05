# Omada Credentials

This page explains the Omada accounts and API credentials needed by OmadaBridge.
There are two different credential types, and they are easy to mix up:

- `OMADA_USER` / `OMADA_PASS` is a normal Omada controller user.
- `OMADA_CLIENT_ID` / `OMADA_SECRET_ID` is an OpenAPI app credential, not a user.

For Omada Fusion, you usually only need `OMADA_USER` and `OMADA_PASS`.

## Which Setup Do I Need?

| Controller type | Required | Optional |
| --- | --- | --- |
| Standard Omada Controller, OC200/OC300/software/cloud-accessed controller | `OMADA_HOST`, `OMADA_USER`, `OMADA_PASS` | `OMADA_CLIENT_ID`, `OMADA_SECRET_ID` for WAN, VPN, ISP, and detailed client metrics |
| Omada Fusion gateway | `OMADA_HOST`, `OMADA_USER`, `OMADA_PASS` | Usually none. Fusion uses web-session OpenAPI automatically. |

Use these defaults unless you know you need to force a mode:

```env
OMADA_SYSTEM_TYPE=auto
OMADA_OPENAPI_AUTH=auto
```

## Find `OMADA_HOST`

Use the same local URL you use in your browser to open the controller.

Examples:

```env
OMADA_HOST=https://192.168.1.20:443
OMADA_HOST=https://192.168.188.1:443
```

If the browser shows a certificate warning for the controller, set:

```env
OMADA_INSECURE=true
```

Prefer a local controller URL over a cloud portal URL when OmadaBridge runs on the same
network or over VPN. Local access is simpler and more reliable for scraping.

## Create a Normal Omada User

This user is used for the controller Web API. OmadaBridge needs it on both standard
controllers and Fusion.

1. Log in to the Omada Controller as the owner or main administrator.
2. Switch to **Global View**.
3. Go to **Account > Role**.
4. Use an existing read-only/viewer-style role, or create a new role.
5. Go to **Account > Account**.
6. Click **Add New User**.
7. Create a user such as `omadabridge` or `exporter`.
8. Set a strong password and save it.
9. Assign the role.
10. In **Site Privileges**, allow the site or sites you want to monitor.
11. Log out and log in once as the new user to confirm the password works.

Put those values into OmadaBridge:

```env
OMADA_USER=omadabridge
OMADA_PASS=change-me
```

Role advice:

- Start with a viewer/read-only role if your controller allows all needed pages.
- If metrics are missing or login works but API calls fail, test with an administrator role.
- After it works, reduce permissions and retest.
- The user must be able to see the target site. If it cannot see the site, OmadaBridge
  cannot resolve `OMADA_SITE`.

## Create OpenAPI Credentials

This applies to standard Omada controllers. Skip this section for Fusion unless your
Fusion firmware later exposes **Open API** app creation.

1. Log in to the Omada Controller as the owner or main administrator.
2. Switch to **Global View**.
3. Go to **Settings > Platform Integration > Open API**.
4. Click **Add New App**.
5. Choose **Client** or **Client Credentials** mode.
6. Name the app, for example `omadabridge`.
7. Select a role and site privilege that can read the monitored site.
8. Save the app.
9. Click the view/eye icon for the app secret.
10. Copy:
    - **Client ID** to `OMADA_CLIENT_ID`
    - **Client Secret** to `OMADA_SECRET_ID`

Example:

```env
OMADA_CLIENT_ID=your-client-id
OMADA_SECRET_ID=your-client-secret
OMADA_OPENAPI_AUTH=auto
```

Do not paste the normal Omada username into `OMADA_CLIENT_ID`. They are unrelated.

## Fusion Notes

Fusion may show only **Webhooks** under **Platform Integration**. That is expected on the
firmware tested with OmadaBridge.

For Fusion, use:

```env
OMADA_HOST=https://192.168.188.1:443
OMADA_USER=omadabridge
OMADA_PASS=change-me
OMADA_SYSTEM_TYPE=auto
OMADA_OPENAPI_AUTH=auto
OMADA_SITE=Default
OMADA_INSECURE=true
```

What happens internally:

- OmadaBridge logs in with `OMADA_USER` and `OMADA_PASS`.
- It detects Fusion automatically.
- It uses Fusion's web-session OpenAPI path.
- It auto-selects Fusion's single hidden site when `OMADA_SITE=Default`.

You do not need `OMADA_CLIENT_ID` or `OMADA_SECRET_ID` for Fusion in this mode.

## If You Cannot Find Open API

Check these first:

- You are in **Global View**, not inside a site.
- You are logged in as an owner/main administrator.
- Your controller firmware/software supports OpenAPI.
- You are opening the local controller UI, not only a limited cloud/mobile view.
- You are not on Fusion firmware that only exposes Webhooks.

If **Platform Integration** only has **Webhooks**:

- On Fusion: omit `OMADA_CLIENT_ID` and `OMADA_SECRET_ID`; keep `OMADA_OPENAPI_AUTH=auto`.
- On a standard controller: update controller firmware or use Web API-only metrics by
  setting `OMADA_OPENAPI_AUTH=disabled`.

## Minimal Docker Examples

Standard controller with OpenAPI:

```yaml
services:
  omada_exporter:
    image: rcooler/omada_exporter:latest
    ports:
      - "9202:9202"
    environment:
      OMADA_HOST: "https://192.168.1.20:443"
      OMADA_USER: "omadabridge"
      OMADA_PASS: "change-me"
      OMADA_CLIENT_ID: "openapi-client-id"
      OMADA_SECRET_ID: "openapi-secret"
      OMADA_SYSTEM_TYPE: "auto"
      OMADA_OPENAPI_AUTH: "auto"
      OMADA_SITE: "Default"
      OMADA_INSECURE: "true"
```

Fusion:

```yaml
services:
  omada_exporter:
    image: rcooler/omada_exporter:latest
    ports:
      - "9202:9202"
    environment:
      OMADA_HOST: "https://192.168.188.1:443"
      OMADA_USER: "omadabridge"
      OMADA_PASS: "change-me"
      OMADA_SYSTEM_TYPE: "auto"
      OMADA_OPENAPI_AUTH: "auto"
      OMADA_SITE: "Default"
      OMADA_INSECURE: "true"
```

## Verify It Works

Start the container, then open:

```text
http://localhost:9202/healthz
http://localhost:9202/metrics
```

Useful check:

```bash
docker logs -f omada_exporter
```

Look for:

- `Login with username ... successful`
- `Received data from devices endpoint`
- `Received data from WAN endpoint`
- `Received data from clients endpoint`

If the exporter starts but some OpenAPI-backed metrics are missing, check logs for
OpenAPI authentication or unsupported endpoint messages.

## Troubleshooting

| Symptom | Likely cause | Fix |
| --- | --- | --- |
| `failed to find site with name Default` | The user cannot see that site, or the site has another name. | Set `OMADA_SITE` to the exact site name, or give the user site privileges. |
| Login fails | Wrong `OMADA_USER` / `OMADA_PASS`, or the account must change password on first login. | Log in manually as that user once, then retry. |
| TLS/certificate error | Controller uses a self-signed certificate. | Set `OMADA_INSECURE=true`. |
| OpenAPI client id/secret rejected | Wrong app credentials, wrong mode, or copied normal username instead. | Create an OpenAPI app in Client Credentials mode and copy Client ID/Secret. |
| Platform Integration only shows Webhooks | Fusion firmware or controller without OpenAPI app support. | For Fusion use `OMADA_OPENAPI_AUTH=auto`; for standard use `disabled` or update firmware. |
| Web API metrics work but WAN/VPN/client details are missing | OpenAPI is disabled or unavailable. | Add OpenAPI app credentials on standard controllers, or check Fusion auto mode. |

