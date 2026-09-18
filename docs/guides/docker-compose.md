# Docker Compose Integration 🐳

BranchBase seamlessly integrates with Docker and Docker Compose workflows.

---

## ⚡ Zero-Config Auto-Detection

When you run `branchbase init`, BranchBase automatically scans for:
- `docker-compose.yml`
- `docker-compose.yaml`
- `compose.yml`
- `compose.yaml`

It automatically extracts:
- **Database service engine:** PostgreSQL (`postgres:16`), MySQL (`mysql:8`), MariaDB (`mariadb:11`).
- **Internal & host port mappings:** e.g., `5432:5432` or `5433:5432`.
- **Environment credentials:** `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `MYSQL_DATABASE`, etc.

---

## 🏗️ Recommended Port Configuration

To use BranchBase's transparent proxy with Docker Compose:

1. Map your Docker container to an internal port (e.g. `5433`):
   ```yaml
   # docker-compose.yml
   version: '3.8'
   services:
     postgres:
       image: postgres:16-alpine
       environment:
         POSTGRES_USER: postgres
         POSTGRES_PASSWORD: secretpassword
         POSTGRES_DB: myapp_dev
       ports:
         - "5433:5432"  # Exposed internally to port 5433
   ```

2. Point BranchBase proxy to forward from standard port `5432` to container port `5433`:
   ```json
   // .branchbase.json
   {
     "driver": "postgres",
     "base_database": "myapp_dev",
     "proxy_port": 5432,
     "target_host": "127.0.0.1",
     "target_port": 5433,
     "username": "postgres",
     "password": "secretpassword"
   }
   ```

3. Run your application querying standard port `5432`:
   ```bash
   branchbase proxy
   ```

BranchBase intercepts connection on port 5432, rewrites the target database to your active branch, and forwards to the Docker container on port 5433!
