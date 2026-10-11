FROM python:3.13-alpine
WORKDIR /demo
COPY server.py test_server.py ./
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
USER 65532:65532
EXPOSE 9100
HEALTHCHECK --interval=5s --timeout=3s --retries=10 CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:9100/healthz', timeout=2)"
CMD ["python", "server.py"]
