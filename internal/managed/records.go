package managed

// Dependency is one dependencies.conf record: a pinned direct download or Git
// checkout.
type Dependency struct{ Kind, Name, Version, Platform, Arch, Source, Checksum, Target, Marker string }

// Resource is one declared managed path: its kind, state name, target and source.
type Resource struct{ Kind, Name, Target, Source string }
