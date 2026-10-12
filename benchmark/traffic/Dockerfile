FROM python:3.13-alpine
WORKDIR /app
COPY client.py ./
COPY traffic/server.py traffic/index.html traffic/app.js traffic/style.css ./traffic/
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 PYTHONPATH=/app
USER 65532:65532
EXPOSE 8080
CMD ["python", "traffic/server.py"]
