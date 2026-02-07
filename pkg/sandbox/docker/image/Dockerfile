FROM python:3.12-slim

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY server.py sandbox_pb2.py sandbox_pb2_grpc.py .

# Use port 8000
EXPOSE 8000

CMD ["python", "server.py"]
