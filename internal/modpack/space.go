package modpack

import (
	"context"
	"os"

	"github.com/UruhaLushia/tomori/internal/storage"
)

func (p *UpdatePlan) AssessSpace(ctx context.Context, root *os.Root, choices map[string]UpdateChoice) (storage.Report, error) {
	if err := updateRootCurrent(root, p.root); err != nil {
		return storage.Report{}, err
	}
	selected, err := p.selections(choices)
	if err != nil {
		return storage.Report{}, err
	}
	var bytes uint64
	for _, index := range selected {
		entry := p.Entry(index)
		next := uint64(max(0, entry.Next.Size))
		bytes = storage.Add(bytes, storage.Add(next, next))
		bytes = storage.Add(bytes, uint64(max(0, entry.CurrentSize)))
	}
	return storage.Assess(ctx, []storage.Requirement{{Directory: root.Name(), Bytes: bytes, Unknown: true}})
}

func (p *MetadataPlan) assessSpace(ctx context.Context, root *os.Root) (storage.Report, error) {
	var bytes uint64
	for _, file := range p.files {
		next := uint64(max(0, file.next.Size))
		bytes = storage.Add(bytes, storage.Add(next, next))
		bytes = storage.Add(bytes, uint64(max(0, file.previous.size)))
	}
	return storage.Assess(ctx, []storage.Requirement{{Directory: root.Name(), Bytes: bytes}})
}
