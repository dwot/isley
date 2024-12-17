# 🌱 Isley - Your Self-Hosted Cannabis Grow Journal

Isley is a self-hosted cannabis grow journal designed to help homegrowers 🌿 track and monitor their plants. With a clean interface and integrations with popular grow equipment, Isley makes managing your grow simple and effective.

I created Isley because it was the tool I wanted but couldn't find. Existing options were limited to phone apps and websites that either didn’t work how I hoped or didn’t work at all. I wanted a single, self-hosted solution to replace:
- 🌡️ Vendor apps for sensor data and graphs.
- 📝 Spreadsheets for seed, harvest, and progress tracking.
- 🗒️ Notepads and memory for feeding/watering history and notes.

Isley doesn't aim to revolutionize your grow. It centralizes your tools into one convenient interface, helping you **track, trend, and elevate your grow**.

For full details, screenshots, and feature highlights, visit our official site 🌐 at [https://isley.dwot.io](https://isley.dwot.io).

---

## 🚀 Key Features

- **📒 Grow Logs**: Track plant growth, watering, and feeding schedules.
- **🌡️ Environmental Monitoring**: View real-time data from grow equipment (AC Infinity, Ecowitt).
- **📸 Image Uploads**: Attach photos to your grow logs for visual tracking.
- **🌱 Seed Inventory**: Manage your seed collection and strain library.
- **📊 Harvest Tracking**: Record harvest details and yields.
- **📈 Graphs and Charts**: Visualize environmental data and plant progress over time.
- **⚙️ Customizable Settings**: Add custom activities and measurements for your grow.
- **📱 Mobile-Friendly**: Works on desktop and mobile devices for convenience.

---

## 🛠️ Features on the Roadmap

- **🌍 Internationalization**: Support for multiple languages.
- **🔔 Alerts and Notifications**: Set custom alerts for environmental conditions.
- **📦 Export and Backup**: Download your grow data for offline storage.
- **📷 Webcam Feeds**: Integrate live webcam feeds for visual monitoring.
- **🗒️ Logging and Debugging**: Improved logging and debugging tools for troubleshooting.

--- 

## 🚀 Quick Start

Isley runs either on **Docker** 🐳 or as a **Windows Executable** 💻. For Docker deployments, it is recommended to use a reverse proxy for production setups to manage external access.

If you don’t already have Docker, follow the [Docker installation instructions](https://docs.docker.com/get-docker/). For `docker-compose`, you can install it [here](https://docs.docker.com/compose/install/).

For Windows, running the executable from the command line allows you to see useful output logs. You can also configure it to run as a service.

---

### 🐳 Option 1: Using Docker Hub (Recommended)

Run Isley directly from the prebuilt Docker image hosted on Docker Hub.

1. **Run Isley Using Docker Compose**:
   Create a `docker-compose.yml` file:

   ```yaml
   version: '3.8'

   services:
     isley:
       image: dwot/isley:latest
       ports:
         - "8080:8080"
       environment:
         - ISLEY_PORT=8080
       volumes:
         - isley-db:/app/data
         - isley-uploads:/app/uploads
       restart: unless-stopped

   volumes:
     isley-db:
     isley-uploads:
   ```

2. **Start the Container**:
   ```bash
   docker-compose up -d
   ```

3. **Access Isley**:
    - Open your browser and go to:
        - `http://localhost:8080` if running locally.
        - `http://<server-ip>:8080` if running remotely.
    - **Default Username**: `admin`  
      **Default Password**: `isley`  
      You will be prompted to change your password on the first login.

4. **Data Persistence**:
   Isley stores all data in the following directories:
    - `/data`: For database storage.
    - `/uploads`: For storing image uploads.

   These directories are mapped to Docker volumes (or bind mounts). Ensure you **do not delete or recreate** these directories during updates. Add them to your **backup process** to prevent data loss.

---

### 💻 Option 2: Using Windows Executable

1. **Download the Executable**:
    - Visit the [Releases Page](https://github.com/dwot/isley/releases) and download the latest `isley.exe` file.

2. **Run Isley**:
    - Open a command prompt and navigate to the folder containing `isley.exe`.
    - Set a custom port (if needed) using the `ISLEY_PORT` environment variable:
      ```bash
      set ISLEY_PORT=8080
      isley.exe
      ```
    - Open your browser and navigate to:
        - `http://localhost:8080` if running locally.
        - `http://<server-ip>:8080` if accessing remotely.

    - **Default Username**: `admin`  
      **Default Password**: `isley`  
      You will be prompted to change your password on the first login.

3. **Data Storage**:
   Isley persists all data in the following directories created alongside the executable:
    - `data/`: For database storage.
    - `uploads/`: For storing image uploads.

   Add these directories to your **backup process** to avoid data loss.

4. **Run as a Service (Optional)**:
    - Use tools like **NSSM** (Non-Sucking Service Manager) to set up Isley as a Windows service:
      ```bash
      nssm install Isley "C:\path\to\isley.exe"
      nssm start Isley
      ```

---

## ⚙️ Configuration

All settings are configurable via the **Settings icon** in the app. You can:

- 🔧 Enable/disable integrations (e.g., AC Infinity, Ecowitt).
- 🔑 Set API keys or server IPs for integrations.
- 🔍 Scan for devices and start data collection.

To override the default port, set the `ISLEY_PORT` environment variable:
```bash
ISLEY_PORT=8080
```

---

## 📝 Notes

- Isley is still in **active development** 🚧. While we strive to avoid breaking changes, improvements are ongoing.
- Found a bug or have suggestions? Report them on the [GitHub repository](https://github.com/dwot/isley/issues).

---

## 🛡️ Recommendations

For production deployments:
- 🐳 Use **Docker** with a reverse proxy (e.g., Nginx, Traefik) to handle external access and TLS.
- 💾 **Backup Directories**:
    - `/data` for database storage.
    - `/uploads` for image uploads.
- 🚫 Avoid deleting or recreating these directories during updates.
- 🔧 Use a Windows service manager to run Isley executable for long-term uptime.

🌐 For more details, screenshots, and the latest updates, visit: [https://isley.dwot.io](https://isley.dwot.io).
