FROM python:3.13-alpine@sha256:bf12ecc0bea3f171d55145dd02c78b5eeb09f30ee2ac9cc2b4c9e4e8f5e30741
WORKDIR /app
COPY client.py ./
COPY traffic/server.py traffic/index.html traffic/app.js traffic/style.css ./traffic/
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 PYTHONPATH=/app
USER 65532:65532
EXPOSE 8080
CMD ["python", "traffic/server.py"]
