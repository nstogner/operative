FROM python:3.12-slim

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY server.py sandbox_pb2.py sandbox_pb2_grpc.py .

# Use port 8000
EXPOSE 8000

# Healthcheck: verify gRPC port is accepting connections.
HEALTHCHECK --interval=2s --timeout=2s --start-period=5s --retries=3 \
    CMD python -c "import socket; s=socket.socket(); s.settimeout(1); s.connect(('127.0.0.1',8000)); s.close()" || exit 1

CMD ["python", "server.py"]

