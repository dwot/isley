# Isley

Isley is a self-hosted cannabis grow journal.

![Isley Dashboard](https://isley.dwot.io/images/dashboard.png)

## Features
 - Integrates with AC Infinity controllers, tracking temperature, humidity and VPD
 - Integrates with Ecowitt soil sensors to track soil moisture

![Isley Sensors](https://isley.dwot.io/images/sensors.png)

- Sensor Data Graphed and presented in a dashboard

![Isley Graphs](https://isley.dwot.io/images/graphs.png)

- Track your grow from seed to harvest
- Notes, Photos, measurements, feedings, waterings, trimmings, trainings, and more all trackable

  ![Isley Plant Data](https://isley.dwot.io/images/isley_plant.png)

- Maintain a seed inventory with strain library
- More features being actively including:
  - Alerting
  - Additional Sensor Sources
  - AC Infinity Device Monitoring
  - Harvest Tracking
  - and more 

# Installation

Isley runs on Docker. To get started, you will need to have Docker installed on your system. If you don’t already have Docker, you can find instructions for installing it [here](https://docs.docker.com/get-docker/). For an easier setup, we also recommend installing Docker Compose, with installation instructions available [here](https://docs.docker.com/compose/install/).

## Quick Start

Follow these steps to install and run Isley:

### Option 1: Using `docker-compose` (Recommended)

1. **Clone the Repository:**
   ```bash
   git clone https://github.com/dwot/isley.git
   cd isley
   ```

2. **Run Isley Using Docker Compose:**
   ```bash
   docker-compose up -d
   ```

3. **Access Isley:**
   Open your web browser and navigate to `http://localhost:8080`.

---

### Option 2: Using `docker run`

If you prefer to run Isley manually using `docker run`, follow these steps:

1. **Clone the Repository:**
   ```bash
   git clone https://github.com/dwot/isley.git
   cd isley
   ```

2. **Build the Docker Image:**
   ```bash
   docker build -t isley .
   ```

3. **Create Persistent Docker Volumes:**
    - Create a volume for the database:
      ```bash
      docker volume create isley-db
      ```
    - Create a volume for uploads:
      ```bash
      docker volume create isley-uploads
      ```

4. **Run the Docker Container:**
   ```bash
   docker run -d -p 8080:8080 -v isley-db:/app/db -v isley-uploads:/app/uploads isley
   ```

5. **Access Isley:**
   Open your web browser and navigate to `http://localhost:8080`.

---

### Notes

- By default, Isley runs on port `8080`. If you need to use a different port, update the `docker-compose.yml` file or modify the `-p` option in the `docker run` command accordingly (e.g., `-p 9090:8080` to use port `9090`).
- Both methods achieve the same result. Using `docker-compose` is simpler and more suitable for most users.
- Make sure you have enough disk space available for the Docker volumes to store data and uploads.

If you encounter any issues during installation or setup, please refer to the documentation or open an issue in the repository.

## Configuration
At this point the only settings are via the Settings icon from the menu in the app. You can enable/disable the AC Infinity and Ecowitt integrations, and set the API keys or Server IP for those integrations.  Once these integrations are set and enabled, two buttons will appear on the sensors page to scan for the devices and start the data collection.

To start tracking a grow, click the Plants icon from the menu and then click Add Plant.  Fill in the details and click Save.  You can now add notes, photos, measurements, feedings, waterings, trimmings, trainings, and more to your plant.

## Notes
Isley is still in development and very much in flux.  While we will endeavor to maintain compatibility, there may be breaking changes as we continue to develop the app.  We'll try not to break anything too badly, but be aware that it could happen.  Report any issues and be patient as we work to develop Isley into a full-featured grow journal.