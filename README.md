# 🌱 Isley - Self-Hosted Cannabis Grow Journal

Isley is a self-hosted cannabis grow journal designed to help homegrowers 🌿 track and monitor their plants. With a clean interface and integrations with popular grow equipment, Isley makes managing your grow simple and effective.

I created Isley because it was the tool I wanted but couldn't find. Existing options were limited to phone apps and websites that either didn’t work how I hoped or didn’t work at all. I wanted a single, self-hosted solution to replace:
- 🌡️ Vendor apps for sensor data and graphs.
- 📝 Spreadsheets for seed, harvest, and progress tracking.
- 🗒️ Notepads and memory for feeding/watering history and notes.

Isley doesn't aim to revolutionize your grow. It centralizes your tools into one convenient interface, helping you **track, trend, and elevate your grow**.

![Isley Dashboard](https://isley.dwot.io/images/dashboard.png?raw=true)

For full details, screenshots, and feature highlights, visit our official site 🌐 at [https://isley.dwot.io](https://isley.dwot.io).

---

## 🚀 Key Features

- **📒 Grow Logs**: Track plant growth, watering, and feeding schedules with custom activity types.
- **🌡️ Environmental Monitoring**: Real-time sensor data from AC Infinity and EcoWitt devices, plus custom ingest via HTTP API.
- **📸 Image Uploads**: Attach photos to plants with captions; supports text overlays and watermarks.
- **📷 Webcam Integration**: Capture periodic snapshots from camera streams via FFmpeg.
- **🌱 Seed Inventory**: Manage strains, breeders, and seed stock with Indica/Sativa and autoflower tracking.
- **📊 Harvest Tracking**: Record harvest dates, yields, and cycle times.
- **📈 Graphs and Charts**: Visualize sensor data over time with configurable retention.
- **⚙️ Customizable Settings**: Define custom zones, activities, metrics, and streams.
- **🌍 Internationalization**: Available in English, German, Spanish, and French.
- **🔓 Guest Mode**: Optional read-only access for unauthenticated visitors.
- **📱 Mobile-Friendly**: Responsive layout for desktop and mobile.

---

## 🛠️ Features on the Roadmap

- **🔔 Alerts and Notifications**: Set custom alerts for environmental conditions.
- **📦 Export and Backup**: Download your grow data for offline storage.

--- 
## 🚀 Quick Start

Isley runs in **Docker** 🐳. SQLite was ideal for early development and lightweight single-container deployments. However, it introduces write contention issues under production loads. **PostgreSQL is now the recommended database backend** for all production deployments.

If you don’t already have Docker, follow the [Docker installation instructions](https://docs.docker.com/get-docker/). For `docker-compose`, you can install it [here](https://docs.docker.com/compose/install/).

---

### 🐳 Option 1: Docker with PostgreSQL (Recommended)

Use the `docker-compose.postgres.yml` file to deploy Isley with a PostgreSQL backend:

1. **Create `docker-compose.postgres.yml`** (or use the provided one):

```yaml
version: '3.8'

services:
  isley:
    image: dwot/isley:latest
    ports:
      - "8080:8080"
    environment:
      - ISLEY_PORT=8080
      - ISLEY_DB_DRIVER=postgres
      - ISLEY_DB_HOST=postgres
      - ISLEY_DB_PORT=5432
      - ISLEY_DB_USER=isley
      - ISLEY_DB_PASSWORD=supersecret
      - ISLEY_DB_NAME=isleydb
    depends_on:
      - postgres
    volumes:
      - isley-uploads:/app/uploads
    restart: unless-stopped

  postgres:
    image: postgres:16
    environment:
      - POSTGRES_DB=isleydb
      - POSTGRES_USER=isley
      - POSTGRES_PASSWORD=supersecret
    volumes:
      - postgres-data:/var/lib/postgresql/data
    restart: unless-stopped

volumes:
  postgres-data:
  isley-uploads:

```

2. **Start the container**:
```bash
docker-compose -f docker-compose.postgres.yml up -d
```

3. **Access Isley**:
- Open your browser:
    - `http://localhost:8080` if running locally.
    - `http://<server-ip>:8080` if running remotely.
- **Default Username**: `admin`  
  **Default Password**: `isley`

You will be prompted to change your password on first login.

---

### 🔄 Optional: Migrate from SQLite to PostgreSQL

If you're upgrading from an existing SQLite-based deployment to PostgreSQL, use the provided `docker-compose.migration.yml` file. This configuration mounts both the existing SQLite volume and the new PostgreSQL data volume into the container.

On startup, **Isley will automatically check**:

- If an existing SQLite database is present in `/app/data/`.
- If the target PostgreSQL instance has no user data.

If both conditions are met, **Isley will import your data from SQLite into PostgreSQL automatically**.

#### 📄 `docker-compose.migration.yml`

```yaml
version: '3.8'

services:
  isley:
    image: dwot/isley:latest
    ports:
      - "8080:8080"
    environment:
      - ISLEY_PORT=8080
      - ISLEY_DB_DRIVER=postgres
      - ISLEY_DB_HOST=postgres
      - ISLEY_DB_PORT=5432
      - ISLEY_DB_USER=isley
      - ISLEY_DB_PASSWORD=supersecret
      - ISLEY_DB_NAME=isleydb
    depends_on:
      - postgres
    volumes:
      - isley-db:/app/data
      - isley-uploads:/app/uploads
    restart: unless-stopped

  postgres:
    image: postgres:16
    environment:
      - POSTGRES_DB=isleydb
      - POSTGRES_USER=isley
      - POSTGRES_PASSWORD=supersecret
    volumes:
      - postgres-data:/var/lib/postgresql/data
    restart: unless-stopped

volumes:
  isley-db:
  postgres-data:
  isley-uploads:

```

After migration, you can switch to `docker-compose.postgres.yml` for your regular production deployment. Be sure to back up your SQLite volume (`isley-db`) before running the migration just in case.

---

### ⚪ Option 2: Docker with SQLite (Legacy)

This method is still available for testing or lightweight local deployments.

1. **Use `docker-compose.sqlite.yml`**:

```yaml
version: '3.8'

services:
  isley:
    image: dwot/isley:latest
    ports:
      - "8080:8080"
    environment:
      - ISLEY_PORT=8080
      - ISLEY_DB_DRIVER=sqlite
    volumes:
      - isley-db:/app/data
      - isley-uploads:/app/uploads
    restart: unless-stopped

volumes:
  isley-db:
  isley-uploads:
```

2. **Start the container**:
```bash
docker-compose -f docker-compose.sqlite.yml up -d
```

3. **Access Isley**:
- Open your browser:
    - `http://localhost:8080` if running locally.
    - `http://<server-ip>:8080` if running remotely.
- **Default Username**: `admin`  
  **Default Password**: `isley`

4. **Data Storage**:
- `/data`: SQLite database storage.
- `/uploads`: Image uploads.

These are mapped via Docker volumes. Add them to your **backup process** to prevent data loss.

> **Note:** This setup is not recommended for production use due to SQLite's limitations with concurrent writes.

---

## ⚙️ Configuration

All settings can be configured via the **Settings icon** in the app. You can:

- 🔧 Enable/disable integrations (e.g., AC Infinity, Ecowitt).
- 🔑 Set API keys or device IPs.
- 🔍 Scan for devices and start data collection.

To override the default port:
```bash
ISLEY_PORT=8080
```

Environment variables for Postgres:
```bash
ISLEY_DB_DRIVER=postgres
ISLEY_DB_HOST=postgres
ISLEY_DB_PORT=5432
ISLEY_DB_USER=isley
ISLEY_DB_PASSWORD=supersecret
ISLEY_DB_NAME=isleydb
```

For SQLite:
```bash
ISLEY_DB_DRIVER=sqlite
```

Additional options:
```bash
ISLEY_DB_FILE=data/isley.db        # SQLite database path
ISLEY_DB_SSLMODE=require           # PostgreSQL SSL mode (require, disable, verify-full, etc.)
ISLEY_SESSION_SECRET=your-secret   # Session encryption key (random per-restart if unset — set this in production)
```

---

## 📝 Notes

- Isley is in **active development** 🚧. Breaking changes may occasionally occur.
- Found a bug or have a suggestion? Open an issue on the [GitHub repository](https://github.com/dwot/isley/issues).

---

## 🛡️ Recommendations

For production:

- 🐳 Use **Docker with PostgreSQL** and a reverse proxy (e.g., Nginx, Traefik) to handle TLS and external access.
- 💾 **Backup Directories/Volumes**:
    - `postgres-data` for PostgreSQL
    - `/uploads` for user content
- ❌ Avoid using SQLite in production.
- 🛠️ Use volume mounts for persistence and scheduled backups.

🌐 For more details, screenshots, and the latest updates, visit: [https://isley.dwot.io](https://isley.dwot.io).

---

## 🔒 Security

- **Login rate limiting**: Login attempts are capped at 5 per minute per IP to prevent brute-force attacks.
- **Session encryption**: Sessions are encrypted using `ISLEY_SESSION_SECRET`. Set this to a long, random string in production — without it, sessions are invalidated on every restart.
- **Security headers**: All responses include `X-Frame-Options`, `X-Content-Type-Options`, `X-XSS-Protection`, and `Referrer-Policy` headers.
- **PostgreSQL SSL**: Defaults to `require`; adjust with `ISLEY_DB_SSLMODE`.
- **API keys**: The ingest endpoint requires an `X-API-KEY` header — generate keys from Settings → API Settings.

---

## 🛡️ API Access and Integrations

Isley now supports programmatic data ingestion via a simple **HTTP API**, secured with API Keys. This allows you to automate sensor readings, integrate with home automation, and more.

### 🔑 API Key Management

To use API endpoints, an **API Key** is required.

**Generating an API Key:**

1. Log in as an admin.
2. Navigate to **Settings** → **API Settings**.
3. Click **Generate New Key**.
4. Copy and store the generated API Key securely.

- Treat your API Key as secret; anyone with the key can push data to your Isley instance.

### 🌐 Ingest Endpoint

The `/api/sensors/ingest` endpoint allows ingestion of environmental, sensor, or activity data into your grow journal.

- **Endpoint:** `/api/sensors/ingest`
- **Method:** `POST`
- **Authentication:** Add your API Key to the header as `X-API-KEY`.

**Payload Example:**
```json
{
  "source": "custom",
  "device": "Arduino Sensor",
  "type": "temperature",
  "value": 25.5,
  "name": "Temperature Sensor 1",
  "new_zone": "Zone Name",
  "unit": "°C"
}
```

> **Tip:** Use this for IoT, custom automations, or integrating off-the-shelf sensors not already supported by Isley.
