# Docker Compose Integration

BranchBase auto-detects database services from Docker Compose configurations and sets up port routing for the transparent proxy.

---

## Auto-Detection during `branchbase init`

When you run `branchbase init`, BranchBase scans for:
- `docker-compose.yml`
- `docker-compose.yaml`
- `compose.yml`
- `compose.yaml`

It automatically extracts:
- **Database service engine:** PostgreSQL (`postgres:16`), MySQL (`mysql:8`), MariaDB (`mariadb:11`).
- **Host port mappings:** e.g., `5433:5432` or `3307:3306`.
- **Environment credentials:** `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `MYSQL_DATABASE`, etc. The MariaDB 10.6+ variables `MARIADB_DATABASE`, `MARIADB_USER`, `MARIADB_PASSWORD`, and `MARIADB_ROOT_PASSWORD` are supported, with the corresponding `MYSQL_*` names retained as fallbacks.

---

## Recommended Port Configuration

To use BranchBase's transparent proxy with Docker Compose:

1. Map your container's database port to an alternative host port (e.g. `5433` on the host to `5432` in the container). This frees the standard host port (`5432`) for the BranchBase proxy:
   ```yaml
   # docker-compose.yml
   services:
     postgres:
       image: postgres:16-alpine
       environment:
         POSTGRES_USER: postgres
         POSTGRES_PASSWORD: secretpassword
         POSTGRES_DB: myapp_dev
       ports:
         - "5433:5432"  # Host port 5433 forwards to container port 5432
   ```

2. Configure BranchBase to listen on standard port `5432` and forward to backend port `5433`:
   ```json
   // .branchbase.json
   {
     "version": "1",
     "driver": "postgres",
     "connection": {
       "host": "127.0.0.1",
       "port": 5433,
       "user": "postgres",
       "password": "secretpassword",
       "base_database": "myapp_dev"
     },
     "proxy": {
       "enabled": true,
       "listen_host": "127.0.0.1",
       "listen_port": 5432,
       "default_branch": "main"
     }
   }
   ```

3. Start the proxy and run your application connecting to standard port `5432`:
   ```bash
   branchbase proxy
   ```

BranchBase intercepts connections on `127.0.0.1:5432`, rewrites the startup handshake with your active branch database name, and streams traffic directly to the backend container on port `5433`.
