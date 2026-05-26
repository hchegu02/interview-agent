# Local BGE-M3 Embedding Server

Small OpenAI-compatible embedding server for local development.

## Install

```powershell
python -m venv tools\bge_server\.venv
tools\bge_server\.venv\Scripts\python.exe -m pip install -r tools\bge_server\requirements.txt
tools\bge_server\.venv\Scripts\python.exe -c "from sentence_transformers import SentenceTransformer; SentenceTransformer('BAAI/bge-m3'); print('loaded')"
```

## Run

```powershell
cd tools\bge_server
.\.venv\Scripts\python.exe -m uvicorn bge_server:app --host 127.0.0.1 --port 8000
```

## Configure interview-agent

```powershell
$env:INTERVIEW_EMBEDDING_MODE="real"
$env:INTERVIEW_EMBEDDING_API_KEY="dummy"
$env:INTERVIEW_EMBEDDING_BASE_URL="http://127.0.0.1:8000/v1"
$env:INTERVIEW_EMBEDDING_MODEL="BAAI/bge-m3"
$env:INTERVIEW_EMBEDDING_DIMENSION="1024"
```

## Reindex

```powershell
go run ./cmd/reindex `
  -seed seeds/question_bank.json `
  -mode real `
  -base-url http://127.0.0.1:8000/v1 `
  -model BAAI/bge-m3 `
  -dim 1024
```
