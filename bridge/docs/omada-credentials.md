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
| Controller in **MSP mode** (one exporter for all MSP customers) | `OMADA_HOST`, `OMADA_USER`, `OMADA_PASS` of an MSP account, `OMADA_CLIENT_ID`, `OMADA_SECRET_ID` of an **MSP-level** Open API app | `OMADA_MSP_CUSTOMERS`, `OMADA_SITE`. See [MSP Mode](#msp-mode). |

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

## MSP Mode

On a controller running in MSP mode, one exporter scrapes **every customer and every
site** the MSP account can see and labels each metric with the customer.

How MSP mode differs:

- Authentication (web login, login check and OpenAPI token) always uses the **MSP ID**.
  Data requests use the **customer ID** instead. The exporter handles this itself; you
  only configure MSP-level credentials.
- One web session and one OpenAPI token are shared by all customers.
- OpenAPI client credentials are **required**. The MSP controller rejects web-session
  OpenAPI (`HTTP 401`, `-44116`), and customer and site discovery uses the OpenAPI.

### MSP account

Use an MSP account with the **MSP Owner** role (the role used for verification), or an
MSP-level role that can see all customers you want to monitor.

```env
OMADA_USER=msp-exporter
OMADA_PASS=change-me
```

### MSP-level Open API app

Create the app in the **MSP view**, not inside a customer:

1. Log in to the controller with the MSP account.
2. Stay in the **MSP** view (the customer list). Do not open a customer.
3. Open the Open API settings (**Settings > Platform Integration > Open API**; menu
   names can differ between controller releases).
4. Click **Add New App** and choose **Client** mode.
5. Give it an MSP-level role that can read every monitored customer.
6. Save, then copy **Client ID** to `OMADA_CLIENT_ID` and **Client Secret** to
   `OMADA_SECRET_ID`.

An app created inside a customer cannot be used: the exporter requests the token with the
MSP ID, and the controller answers `-44106 Client Id Or Client Secret is Invalid`. If you
see that error in MSP mode, create the app at MSP level.

### Example

```env
OMADA_HOST=https://omada.example.net:8043
OMADA_USER=msp-exporter
OMADA_PASS=change-me
OMADA_CLIENT_ID=msp-level-client-id
OMADA_SECRET_ID=msp-level-client-secret
OMADA_SYSTEM_TYPE=auto          # detects MSP mode; or set msp explicitly
OMADA_OPENAPI_AUTH=auto
# Optional filters:
# OMADA_MSP_CUSTOMERS=Alpha Corp,7d2c1b...   # names or IDs; default: all
# OMADA_SITE=Default,HQ                      # names or IDs; default: all sites
```

The exporter fails at startup in MSP mode when `OMADA_CLIENT_ID` or `OMADA_SECRET_ID` is
missing, or when `OMADA_OPENAPI_AUTH` is `disabled` or `web_session`.

See [MSP mode in the installation guide](./installation.md#msp-mode) for every MSP
option, the labels, and the limitations.

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
| `controllerStatus returned errorCode -1600: Unsupported request path` | The controller release does not expose the controller-status Web API used by OmadaBridge; this is known on older firmware. | Verify the controller URL and system type, then upgrade the controller. Other collectors may continue, but controller metrics and related dashboard panels will show no data. |
| Platform Integration only shows Webhooks | Fusion firmware or controller without OpenAPI app support. | For Fusion use `OMADA_OPENAPI_AUTH=auto`; for standard use `disabled` or update firmware. |
| Web API metrics work but WAN/VPN/client details are missing | OpenAPI is disabled or unavailable. | Add OpenAPI app credentials on standard controllers, or check Fusion auto mode. |
| `current user returned errorCode -1: General error.` at startup | The controller is in MSP mode but the exporter runs in single-site mode (an older release, or `OMADA_SYSTEM_TYPE=standard`/`fusion`). | Use `OMADA_SYSTEM_TYPE=auto` or `msp` with MSP-level OpenAPI credentials. |
| `MSP OpenAPI login ... errorCode -44106` | The Open API app was created inside a customer, not at MSP level, or the secret is wrong. | Create a Client-mode app in the MSP view. See [MSP Mode](#msp-mode). |
| `MSP mode requires OMADA_CLIENT_ID and OMADA_SECRET_ID` | MSP mode was detected or forced without OpenAPI credentials. | Add MSP-level app credentials. MSP controllers reject web-session OpenAPI (`-44116`). |
| `omada_msp_target_up` is `0` for one customer | The MSP account or app cannot read that customer (often `-1005 Operation forbidden`). | Check the MSP role and the app's customer privileges. Other customers are not affected. |
