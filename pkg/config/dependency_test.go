package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {

	pwd, _ := os.Getwd()
	target := NewDependency(pwd, false)

	actual, err := target.Load()
	require.NoError(t, err)
	require.Equal(t, 2, len(actual.Dependencies))

	withBranch := actual.Dependencies[0]
	withRevision := actual.Dependencies[1]

	require.Equal(t, "github.com/protocolbuffers/protobuf/src", withBranch.Target)
	require.Equal(t, "master", withBranch.Branch)
	require.Equal(t, "", withBranch.Revision)
	require.Equal(t, "", withBranch.Protocol)

	require.Equal(t, "github.com/grpc-ecosystem/grpc-gateway/examples/internal/helloworld", withRevision.Target)
	require.Equal(t, "", withRevision.Branch)
	require.Equal(t, "v2.7.2", withRevision.Revision)
	require.Equal(t, "grpc-gateway/examples/internal/helloworld", withRevision.Path)
	require.Equal(t, "ssh", withRevision.Protocol)
}

// TestLoadBehavior_NoLockFile tests that without a lock file, dependencies are loaded from toml
func TestLoadBehavior_NoLockFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create only protodep.toml with 2 dependencies
	tomlContent := `proto_outdir = "./proto"

[[dependencies]]
  target = "github.com/example/repo1"
  branch = "main"

[[dependencies]]
  target = "github.com/example/repo2"
  revision = "v1.0.0"
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "protodep.toml"), []byte(tomlContent), 0644))

	// Without lock file, forceUpdate=false should still read from toml
	dep := NewDependency(tmpDir, false)
	require.True(t, dep.IsNeedWriteLockFile(), "should need to write lock file when no lock exists")

	protodep, err := dep.Load()
	require.NoError(t, err)
	require.Equal(t, 2, len(protodep.Dependencies))
	require.Equal(t, "github.com/example/repo1", protodep.Dependencies[0].Target)
	require.Equal(t, "github.com/example/repo2", protodep.Dependencies[1].Target)
}

// TestLoadBehavior_WithLockFile_NoForce tests that with a lock file and no force,
// dependencies are loaded ONLY from lock file - new deps in toml are ignored
func TestLoadBehavior_WithLockFile_NoForce(t *testing.T) {
	tmpDir := t.TempDir()

	// Create protodep.toml with 3 dependencies (including a new one)
	tomlContent := `proto_outdir = "./proto"

[[dependencies]]
  target = "github.com/example/repo1"
  branch = "main"

[[dependencies]]
  target = "github.com/example/repo2"
  revision = "v1.0.0"

[[dependencies]]
  target = "github.com/example/repo3-new"
  branch = "develop"
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "protodep.toml"), []byte(tomlContent), 0644))

	// Create protodep.lock with only 2 dependencies (locked revisions)
	lockContent := `proto_outdir = "./proto"

[[dependencies]]
  target = "github.com/example/repo1"
  branch = "main"
  revision = "abc123locked"

[[dependencies]]
  target = "github.com/example/repo2"
  revision = "v1.0.0"
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "protodep.lock"), []byte(lockContent), 0644))

	// With lock file and forceUpdate=false, should read from lock file only
	dep := NewDependency(tmpDir, false)
	require.False(t, dep.IsNeedWriteLockFile(), "should NOT need to write lock file when lock exists and no force")

	protodep, err := dep.Load()
	require.NoError(t, err)

	// KEY ASSERTION: Only 2 deps from lock file, repo3-new from toml is ignored!
	require.Equal(t, 2, len(protodep.Dependencies),
		"without force, new dependencies in toml are ignored - only lock file deps are loaded")
	require.Equal(t, "github.com/example/repo1", protodep.Dependencies[0].Target)
	require.Equal(t, "abc123locked", protodep.Dependencies[0].Revision,
		"locked revision should be preserved")
	require.Equal(t, "github.com/example/repo2", protodep.Dependencies[1].Target)
}

// TestLoadBehavior_WithLockFile_Force tests that with force flag,
// ALL dependencies are read from toml (losing locked revisions)
func TestLoadBehavior_WithLockFile_Force(t *testing.T) {
	tmpDir := t.TempDir()

	// Create protodep.toml with 3 dependencies
	tomlContent := `proto_outdir = "./proto"

[[dependencies]]
  target = "github.com/example/repo1"
  branch = "main"

[[dependencies]]
  target = "github.com/example/repo2"
  revision = "v1.0.0"

[[dependencies]]
  target = "github.com/example/repo3-new"
  branch = "develop"
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "protodep.toml"), []byte(tomlContent), 0644))

	// Create protodep.lock with only 2 dependencies (with locked revisions)
	lockContent := `proto_outdir = "./proto"

[[dependencies]]
  target = "github.com/example/repo1"
  branch = "main"
  revision = "abc123locked"

[[dependencies]]
  target = "github.com/example/repo2"
  revision = "v1.0.0"
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "protodep.lock"), []byte(lockContent), 0644))

	// With forceUpdate=true, should read from toml
	dep := NewDependency(tmpDir, true)
	require.True(t, dep.IsNeedWriteLockFile(), "should need to write lock file when force is set")

	protodep, err := dep.Load()
	require.NoError(t, err)

	// KEY ASSERTION: All 3 deps from toml, but repo1 loses its locked revision!
	require.Equal(t, 3, len(protodep.Dependencies),
		"with force, all dependencies from toml are loaded including new ones")
	require.Equal(t, "github.com/example/repo1", protodep.Dependencies[0].Target)
	require.Equal(t, "", protodep.Dependencies[0].Revision,
		"with force, the locked revision is LOST - repo1 will resolve to latest on branch")
	require.Equal(t, "github.com/example/repo3-new", protodep.Dependencies[2].Target,
		"new dependency from toml is now included")
}

// TestLoadBehavior_NoMiddleGround documents that there's no way to:
// - Add new dependencies from toml
// - While preserving locked revisions for existing deps
func TestLoadBehavior_NoMiddleGround(t *testing.T) {
	tmpDir := t.TempDir()

	tomlContent := `proto_outdir = "./proto"

[[dependencies]]
  target = "github.com/example/existing"
  branch = "main"

[[dependencies]]
  target = "github.com/example/new-dep"
  branch = "develop"
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "protodep.toml"), []byte(tomlContent), 0644))

	lockContent := `proto_outdir = "./proto"

[[dependencies]]
  target = "github.com/example/existing"
  branch = "main"
  revision = "locked-hash-12345"
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "protodep.lock"), []byte(lockContent), 0644))

	// Option 1: No force - new dep is ignored
	depNoForce := NewDependency(tmpDir, false)
	protodepNoForce, _ := depNoForce.Load()
	require.Equal(t, 1, len(protodepNoForce.Dependencies),
		"without force: new-dep is NOT added")
	require.Equal(t, "locked-hash-12345", protodepNoForce.Dependencies[0].Revision,
		"without force: locked revision is preserved")

	// Option 2: With force - locked revision is lost
	depForce := NewDependency(tmpDir, true)
	protodepForce, _ := depForce.Load()
	require.Equal(t, 2, len(protodepForce.Dependencies),
		"with force: new-dep IS added")
	require.Equal(t, "", protodepForce.Dependencies[0].Revision,
		"with force: locked revision is LOST (will resolve to latest)")

	// There is no middle ground option that would:
	// - Include new-dep (from toml)
	// - AND preserve locked-hash-12345 for existing dep
}
