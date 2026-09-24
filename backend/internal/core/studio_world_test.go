package core

import "testing"

func studioSpecFixture() StudioWorldSpec {
	return StudioWorldSpec{Version: StudioWorldSpecVersion, Name: "新世界", StartWorldTime: StudioWorldStart, Population: 20, OpeningMoneyMinor: 10000, OpeningStockMinor: 100, Places: []StudioWorldPlace{{"home", "住所", "home"}, {"square", "广场", "public"}}, Links: []StudioWorldLink{{"home", "square", 5}}, People: []StudioWorldPerson{{"lin", "Lin", "home", true}, {"cai", "Cai", "square", false}}}
}

func TestStudioWorldSpecificationBoundsAndTopology(t *testing.T) {
	if err := studioSpecFixture().Validate(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*StudioWorldSpec){
		"version": func(s *StudioWorldSpec) { s.Version = "other" }, "calendar": func(s *StudioWorldSpec) { s.StartWorldTime = "2027-01-01T00:00:00Z" },
		"unsafe-money": func(s *StudioWorldSpec) { s.OpeningMoneyMinor = MaxJSONSafeInteger + 1 }, "negative-stock": func(s *StudioWorldSpec) { s.OpeningStockMinor = -1 },
		"population": func(s *StudioWorldSpec) { s.Population = 1 }, "duplicate-place": func(s *StudioWorldSpec) { s.Places[1].Key = "home" },
		"unknown-link": func(s *StudioWorldSpec) { s.Links[0].To = "missing" }, "self-link": func(s *StudioWorldSpec) { s.Links[0].To = "home" },
		"duplicate-link": func(s *StudioWorldSpec) { s.Links = append(s.Links, StudioWorldLink{"square", "home", 2}) },
		"disconnected":   func(s *StudioWorldSpec) { s.Places = append(s.Places, StudioWorldPlace{"remote", "远处", "work"}) },
		"unknown-place":  func(s *StudioWorldSpec) { s.People[0].Place = "missing" }, "two-players": func(s *StudioWorldSpec) { s.People[1].Player = true },
		"no-player": func(s *StudioWorldSpec) { s.People[0].Player = false }, "duplicate-person": func(s *StudioWorldSpec) { s.People[1].Key = "lin" },
		"bad-name": func(s *StudioWorldSpec) { s.Name = "\x00" }, "bad-key": func(s *StudioWorldSpec) { s.People[0].Key = "../lin" },
	} {
		t.Run(name, func(t *testing.T) {
			s := studioSpecFixture()
			change(&s)
			if !HasCode(s.Validate(), CodeInvalidArgument) {
				t.Fatal("invalid world accepted")
			}
		})
	}
}

func TestStudioWorldIdentityIsStableAndNamespaced(t *testing.T) {
	a, err := StudioWorldObjectID("world-a", "entity", "lin")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := StudioWorldObjectID("world-a", "entity", "lin")
	if a != b {
		t.Fatal("unstable identity")
	}
	for _, parts := range [][3]string{{"world-b", "entity", "lin"}, {"world-a", "place", "lin"}, {"world-a", "entity", "cai"}} {
		b, err := StudioWorldObjectID(parts[0], parts[1], parts[2])
		if err != nil || a == b {
			t.Fatal("identity collision", err)
		}
	}
	if _, err := StudioWorldObjectID("", "entity", "lin"); err == nil {
		t.Fatal("empty world accepted")
	}
	s := studioSpecFixture()
	before, _ := HashJSON(s)
	s.OpeningStockMinor++
	after, _ := HashJSON(s)
	if before == after {
		t.Fatal("settings omitted from request hash")
	}
}
