import logging
import os
import signal
import socket
import sys
import threading
import time

import requests
from flask import Flask, render_template

app = Flask(__name__)
app.config["TEMPLATES_AUTO_RELOAD"] = True


class HealthCheckFilter(logging.Filter):
    """Drop Werkzeug access log lines for Consul's /health checks."""

    def filter(self, record: logging.LogRecord) -> bool:
        return "GET /health " not in record.getMessage()


logging.getLogger("werkzeug").addFilter(HealthCheckFilter())

SERVICE_NAME = "mock"
SERVICE_PORT = 8888
CONSUL_ADDR = os.getenv("CONSUL_ADDR", "consul")
CONSUL_PORT = os.getenv("CONSUL_PORT", "8500")

# Unique per container; used as the Consul service instance ID.
instance_id = socket.gethostname()


def container_ip() -> str:
    """Return this container's IP on the docker network."""
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        # UDP connect never sends packets; it just selects the route.
        s.connect(("8.8.8.8", 80))
        return s.getsockname()[0]
    finally:
        s.close()


def agent_url() -> str:
    return f"http://{CONSUL_ADDR}:{CONSUL_PORT}/v1/agent"


def register() -> bool:
    payload = {
        "ID": instance_id,
        "Name": SERVICE_NAME,
        "Address": container_ip(),
        "Port": SERVICE_PORT,
        "Tags": [
            "traefik.enable=true",
        ],
        "Check": {
            "HTTP": f"http://{container_ip()}:{SERVICE_PORT}/health",
            "Interval": "10s",
            "Timeout": "2s",
            "DeregisterCriticalServiceAfter": "30s",
        },
    }
    try:
        resp = requests.put(f"{agent_url()}/service/register", json=payload, timeout=5)
        ok = resp.status_code == 200
    except requests.RequestException:
        ok = False
    if ok:
        print(f"registered with Consul as {instance_id} ({payload['Address']})", flush=True)
    return ok


def deregister() -> None:
    try:
        requests.put(f"{agent_url()}/service/deregister/{instance_id}", timeout=5)
        print(f"deregistered {instance_id} from Consul", flush=True)
    except requests.RequestException as exc:
        print(f"deregistration failed: {exc}", flush=True)


def register_loop() -> None:
    while not register():
        time.sleep(5)


def handle_signal(signum, frame):
    deregister()
    sys.exit(0)


threading.Thread(target=register_loop, daemon=True).start()
signal.signal(signal.SIGTERM, handle_signal)
signal.signal(signal.SIGINT, handle_signal)


@app.route("/health")
def health():
    return "ok"


@app.route("/")
def hello_world():
    return render_template("index.html", instance_id=instance_id)

@app.route("/doOperation")
def doOperation():
    return render_template("doOperation.html", instance_id=instance_id)


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=SERVICE_PORT, debug=False)
