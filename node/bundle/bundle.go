package bundle

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/ipld/go-car"

	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"

	cid "github.com/ipfs/go-cid"
)

func LoadBundleFromFile(ctx context.Context, bs blockstore.Blockstore, path string) (cid.Cid, error) {
	f, err := os.Open(path)
	if err != nil {
		return cid.Undef, xerrors.Errorf("error opening bundle %q for builtin-actors: %w", path, err)
	}
	defer f.Close() //nolint

	return LoadBundle(ctx, bs, f)
}

func LoadBundle(ctx context.Context, bs blockstore.Blockstore, r io.Reader) (cid.Cid, error) {
	hdr, err := car.LoadCar(ctx, bs, r)
	if err != nil {
		return cid.Undef, xerrors.Errorf("error loading builtin actors bundle: %w", err)
	}

	if len(hdr.Roots) != 1 {
		return cid.Undef, xerrors.Errorf("expected one root when loading actors bundle, got %d", len(hdr.Roots))
	}
	return hdr.Roots[0], nil
}

// LoadBundles loads the bundles for the specified actor versions into the passed blockstore, if and
// only if the bundle's manifest is not already present in the blockstore.
func LoadBundles(ctx context.Context, bs blockstore.Blockstore, versions ...actors.Version) error {
	netw := build.NetworkBundle

	for _, av := range versions {
		// No bundles before version 8.
		if av < actors.Version8 {
			continue
		}

		manifestCid, ok := actors.GetManifest(av)
		if !ok {
			// All manifests are registered on start, so this must succeed.
			return xerrors.Errorf("unknown actor version v%d", av)
		}

		if haveManifest, err := bs.Has(ctx, manifestCid); err != nil {
			return xerrors.Errorf("blockstore error when loading manifest %s: %w", manifestCid, err)
		} else if haveManifest {
			// We already have the manifest, and therefore everything under it.
			continue
		}

		bd, ok := build.BuiltinActorReleases[av]
		if !ok {
			return xerrors.Errorf("unknown actors version %d", av)
		}

		envvar := fmt.Sprintf("LOTUS_BUILTIN_ACTORS_V%d_BUNDLE", av)
		var (
			root cid.Cid
			err  error
		)
		if path := os.Getenv(envvar); path != "" {
			root, err = LoadBundleFromFile(ctx, bs, path)
		} else if path = bd.Path[netw]; path != "" {
			root, err = LoadBundleFromFile(ctx, bs, path)
		} else if embedded, ok := build.GetEmbeddedBuiltinActorsBundle(av); ok {
			root, err = LoadBundle(ctx, bs, bytes.NewReader(embedded))
		} else {
			err = xerrors.Errorf("bundle for actors version v%d not found", av)
		}

		if err != nil {
			return err
		}

		if root != manifestCid {
			return xerrors.Errorf("expected manifest for actors version %d does not match actual: %s != %s", av, manifestCid, root)
		}

	}

	return nil
}
