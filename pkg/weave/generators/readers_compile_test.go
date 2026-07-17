package generators

import (
	"testing"

	"github.com/pletka-io/pletka/pkg/weave/collection"
	fieldsvc "github.com/pletka-io/pletka/pkg/weave/field"
	"github.com/pletka-io/pletka/pkg/weave/model"
	"github.com/pletka-io/pletka/pkg/weave/namespacebinding"
	"github.com/pletka-io/pletka/pkg/weave/project"
)

func TestSliceServicesSatisfyGeneratorReaders(t *testing.T) {
	var _ ProjectReader = (*project.Service)(nil)
	var _ ModelReader = (*model.Service)(nil)
	var _ CollectionReader = (*collection.Service)(nil)
	var _ FieldReader = (*fieldsvc.Service)(nil)
	var _ NamespaceReader = (*namespacebinding.Service)(nil)
}
