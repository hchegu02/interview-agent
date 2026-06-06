package questionbank

func cloneImportJob(job ImportJob) ImportJob {
	job.Metadata = cloneStringMap(job.Metadata)
	return job
}

func cloneImportChunk(chunk ImportChunk) ImportChunk {
	chunk.Metadata = cloneStringMap(chunk.Metadata)
	return chunk
}

func cloneImportItem(item ImportItem) ImportItem {
	item.Item = cloneItem(item.Item)
	if item.OriginalItem != nil {
		original := cloneItem(*item.OriginalItem)
		item.OriginalItem = &original
	}
	item.FieldProvenance = cloneStringMap(item.FieldProvenance)
	item.SourceProvenance = cloneStringMap(item.SourceProvenance)
	item.Errors = append([]string(nil), item.Errors...)
	return item
}

func cloneImportItems(items []Item) []Item {
	if len(items) == 0 {
		return nil
	}
	out := make([]Item, 0, len(items))
	for _, item := range items {
		out = append(out, cloneItem(item))
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
