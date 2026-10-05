//go:build integration

package project_test

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pletka-io/pletka/pkg/weave"
	"github.com/pletka-io/pletka/pkg/weave/collection"
	"github.com/pletka-io/pletka/pkg/weave/model"
	"github.com/pletka-io/pletka/pkg/weave/override"
	"github.com/pletka-io/pletka/pkg/weave/release"
)

// project.Host requires the three readers that back the adoption picker.
// They are required rather than optional on purpose: a nil reader would
// return the picker to listing another project's draft entities, which is
// the defect the picker's release scoping exists to prevent, so Host
// refuses to mount without them.
//
// The save-conflict tests below do not exercise the picker, but they build a
// real Host, so they need real readers. These helpers build them from the
// same pool and in the same shape as pkg/app's buildCoreEntityHosts, which
// keeps those tests' claim to reproduce the app's wiring true.

func releaseReaderForTest(pool *pgxpool.Pool) *release.Service {
	return release.NewService(release.NewPostgresStore(pool), pool, nil)
}

func adoptionReadersForTest(pool *pgxpool.Pool) (*model.Service, *collection.Service, *release.Service) {
	weaveStore := weave.NewPostgresStore(pool)
	overrideSvc := override.NewService(override.NewPostgresStore(pool), nil, nil)
	projects := weaveStore.Projects()
	categories := weaveStore.WeaveCategories()

	modelSvc := model.NewService(
		model.NewPostgresStore(pool), overrideSvc,
		weaveStore.Adoptions(), weaveStore.Forks(),
		projects, projects, weaveStore, weaveStore, categories, nil, nil,
	)
	collectionSvc := collection.NewService(
		collection.NewPostgresStore(pool), overrideSvc,
		weaveStore.Adoptions(), weaveStore.Forks(),
		projects, projects, weaveStore, weaveStore, categories, nil, nil,
	)
	return modelSvc, collectionSvc, releaseReaderForTest(pool)
}
