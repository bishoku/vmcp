"""Broker clients must never be linked into either request-serving binary."""
from pathlib import Path
import subprocess

core = Path(__file__).resolve().parents[1] / 'core'
for binary in ('control-plane', 'data-plane', 'audit-exporter'):
    deps = subprocess.check_output(['go', 'list', '-deps', './cmd/' + binary], cwd=core, text=True)
    forbidden = [line for line in deps.splitlines() if line.startswith(('github.com/twmb/', 'github.com/rabbitmq/', 'github.com/bishoku/vmcp/core/internal/auditkafka', 'github.com/bishoku/vmcp/core/internal/auditrabbitmq'))]
    if forbidden:
        raise SystemExit(f'{binary} unexpectedly imports broker dependencies: {forbidden}')
print('Control plane, data plane and HTTP exporter have no Kafka/RabbitMQ dependencies')
