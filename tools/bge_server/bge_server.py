from __future__ import annotations

import os
from typing import Annotated

from fastapi import FastAPI, Header, HTTPException
from pydantic import BaseModel


MODEL_NAME = "BAAI/bge-m3"
VECTOR_DIMENSION = 1024

os.environ.setdefault("HF_HUB_OFFLINE", "1")
os.environ.setdefault("TRANSFORMERS_OFFLINE", "1")

from sentence_transformers import SentenceTransformer

app = FastAPI(title="Local BGE-M3 Embedding Server")
model = SentenceTransformer(MODEL_NAME)


class EmbeddingRequest(BaseModel):
    model: str
    input: list[str]
    dimensions: int | None = None


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"status": "ok", "model": MODEL_NAME}


@app.post("/v1/embeddings")
def embeddings(
    req: EmbeddingRequest,
    authorization: Annotated[str | None, Header()] = None,
) -> dict[str, object]:
    if req.model != MODEL_NAME:
        raise HTTPException(status_code=400, detail=f"unsupported model: {req.model}")
    if not req.input:
        raise HTTPException(status_code=400, detail="input must not be empty")
    if req.dimensions is not None and req.dimensions != VECTOR_DIMENSION:
        raise HTTPException(
            status_code=400,
            detail=f"unsupported dimensions: {req.dimensions}, want {VECTOR_DIMENSION}",
        )

    vectors = model.encode(
        req.input,
        normalize_embeddings=True,
        convert_to_numpy=True,
    )

    data = []
    for index, vector in enumerate(vectors):
        embedding = vector.astype(float).tolist()
        if len(embedding) != VECTOR_DIMENSION:
            raise HTTPException(
                status_code=500,
                detail=f"vector dim {len(embedding)}, want {VECTOR_DIMENSION}",
            )
        data.append(
            {
                "object": "embedding",
                "index": index,
                "embedding": embedding,
            }
        )

    return {
        "object": "list",
        "data": data,
        "model": MODEL_NAME,
        "usage": {
            "prompt_tokens": 0,
            "total_tokens": 0,
        },
    }
