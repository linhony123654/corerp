package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func retailAuthorBundle(t *testing.T) StudioPackageBundle {
	t.Helper()
	base := "../../../docs/f9/retail-career"
	manifest, err := os.ReadFile(filepath.Join(base, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(base, "content.json"))
	if err != nil {
		t.Fatal(err)
	}
	var bundle StudioPackageBundle
	if err := json.Unmarshal(manifest, &bundle.Manifest); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(content, &bundle.Content); err != nil {
		t.Fatal(err)
	}
	return bundle
}

func TestF9RetailAuthorBundleValidatedAsReferences(t *testing.T) {
	bundle := retailAuthorBundle(t)
	if err := bundle.Validate(); err != nil {
		t.Fatalf("authored retail content invalid: %v", err)
	}
	job := bundle.Content.RetailCareer.Jobs[0]
	if bundle.Manifest.Kind != "content" || job.WageReferenceMinor != 16 || job.RequiredCredential.Code != "retail_customer_service" || job.Training.MinimumMinutes != 60 || job.WorkStartHour != 8 || job.WorkEndHour != 17 || job.NextGrade != "senior" {
		t.Fatal("retail author fields lost", bundle)
	}
	for name, change := range map[string]func(*StudioPackageBundle){
		"illegal capability": func(b *StudioPackageBundle) { b.Manifest.Capabilities = []string{"world.money.write"} },
		"hash mismatch":      func(b *StudioPackageBundle) { b.Content.RetailCareer.Jobs[0].WageReferenceMinor++ },
		"wrong kind":         func(b *StudioPackageBundle) { b.Manifest.Kind = "system" },
		"extra executable":   func(b *StudioPackageBundle) { b.Manifest.ContentFiles = append(b.Manifest.ContentFiles, "script.js") },
		"invalid shift": func(b *StudioPackageBundle) {
			b.Content.RetailCareer.Jobs[0].WorkEndHour = 7
			b.Manifest.ContentHash, _ = HashJSON(b.Content)
		},
		"invalid wage": func(b *StudioPackageBundle) {
			b.Content.RetailCareer.Jobs[0].WageReferenceMinor = 0
			b.Manifest.ContentHash, _ = HashJSON(b.Content)
		},
		"duplicate job": func(b *StudioPackageBundle) {
			b.Content.RetailCareer.Jobs = append(b.Content.RetailCareer.Jobs, b.Content.RetailCareer.Jobs[0])
			b.Manifest.ContentHash, _ = HashJSON(b.Content)
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := retailAuthorBundle(t)
			change(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("invalid retail bundle accepted")
			}
		})
	}
}
