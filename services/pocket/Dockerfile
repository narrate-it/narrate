# Supply a reviewed ARM64 Python 3.12 base image by immutable digest.
ARG BASE_IMAGE
FROM ${BASE_IMAGE}
WORKDIR /app
COPY requirements.txt .
RUN python -m pip install --no-cache-dir -r requirements.txt
COPY server.py /app/server.py
ENTRYPOINT ["python", "/app/server.py"]
