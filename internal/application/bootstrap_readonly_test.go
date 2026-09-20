package application

import (
	"testing"
)

func TestBootstrapDoesNotRecreateDefaultDirectory(t *testing.T) {
	service, ctx, first, _ := newOnboardedService(t, "bootstrap-readonly", []string{"Alice"})
	if len(first.Institutions) == 0 || len(first.Groups) == 0 {
		t.Fatal("onboarding should create default directory entries")
	}
	second, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Institutions) != len(first.Institutions) || len(second.Groups) != len(first.Groups) {
		t.Fatalf("bootstrap mutated directory: institutions %d->%d groups %d->%d",
			len(first.Institutions), len(second.Institutions), len(first.Groups), len(second.Groups))
	}
	if err := service.ArchiveInstitution(ctx, first.Institutions[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if err := service.ArchiveGroup(ctx, first.Groups[0].ID, true); err != nil {
		t.Fatal(err)
	}
	third, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(third.Institutions) != 0 || len(third.Groups) != 0 {
		t.Fatalf("bootstrap recreated defaults: institutions=%d groups=%d", len(third.Institutions), len(third.Groups))
	}
}
