def prepare_results(docs, min_score, top_k):
    filtered_docs = [doc for doc in docs if doc["score"] >= min_score]
    filtered_docs.sort(key=lambda x: x["score"], reverse=True)
    seen_ids = set()

    deduped_docs = []
    for doc in filtered_docs:
        doc_id = doc["id"]
        if doc_id not in seen_ids:
            seen_ids.add(doc_id)
            deduped_docs.append(doc)

    return deduped_docs[:top_k]

if __name__ == "__main__":
    docs = [
        {"id": "a", "score": 0.6},
        {"id": "b", "score": 0.9},
        {"id": "a", "score": 0.95},
        {"id": "c", "score": 0.8},
        {"id": "d", "score": 0.8},
        {"id": "b", "score": 0.7},
    ]

    print(prepare_results(docs, 0.8, 3))
    print(prepare_results(docs, 0.8, 10))
    print(prepare_results(docs, 0.8, 0))
    print(prepare_results(docs, 1.0, 3 ))
