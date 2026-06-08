FROM python:3.11-slim

ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1
ENV HF_HOME=/home/app/.cache/huggingface

RUN useradd -m -u 10001 app

WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY bge_server.py .
RUN mkdir -p /home/app/.cache/huggingface \
    && chown -R app:app /home/app

USER app
EXPOSE 8000

CMD ["python", "-m", "uvicorn", "bge_server:app", "--host", "0.0.0.0", "--port", "8000"]
