package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/ioutil"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/middleware"
	"github.com/shibukawa/configdir"
)

var confChecksum string

func readConfJson(ctx context.Context, cancel context.CancelFunc) (confB []byte, err error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	for _, scope := range []configdir.ConfigType{configdir.Global, configdir.System} {
		conf := configdir.New("tupelo", filepath.Join(configNamespace, "app"))
		folders := conf.QueryFolders(scope)
		fpath := filepath.Join(folders[0].Path, "conf.json")
		middleware.Log.Debugw("trying to load configuration from file", "filename", fpath)
		if err = watcher.Add(fpath); err != nil {
			middleware.Log.Debugw("loading configuration from file failed", "filename", fpath,
				"error", err)
			continue
		}
		confB, err = ioutil.ReadFile(fpath)
		if err != nil {
			middleware.Log.Debugw("loading configuration from file failed", "filename", fpath,
				"error", err)
			if err = watcher.Remove(fpath); err != nil {
				return
			}
			continue
		}

		middleware.Log.Infow("loading configuration from file", "filename", fpath)
		h := sha256.New()
		if _, err = h.Write(confB); err != nil {
			return
		}
		confChecksum = hex.Dump(h.Sum(nil))

		launchMonitor(ctx, cancel, watcher)
		return
	}

	watcher.Close()
	return
}

func launchMonitor(ctx context.Context, cancel context.CancelFunc, watcher *fsnotify.Watcher) {
	go func() {
		middleware.Log.Infow("monitoring configuration file...")
		for {
			select {
			case <-ctx.Done():
				middleware.Log.Debugw("ending configuration file monitoring since context is done")
				return
			case event, ok := <-watcher.Events:
				if !ok {
					middleware.Log.Debugw("fsnotify events channel closed, exiting")
					return
				}

				middleware.Log.Debugw("received fsnotify event", "op", event.Op, "filename", event.Name)

				confB, err := ioutil.ReadFile(event.Name)
				if err != nil {
					middleware.Log.Errorw("loading configuration from file failed", "filename", event.Name,
						"error", err)
					continue
				}

				h := sha256.New()
				if _, err = h.Write(confB); err != nil {
					middleware.Log.Errorw("checksumming configuration failed", "error", err)
					continue
				}
				checksum := hex.Dump(h.Sum(nil))
				if checksum != confChecksum {
					middleware.Log.Debugw("configuration file is modified, restarting",
						"filename", event.Name)
					cancel()
					return
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					middleware.Log.Debugw("fsnotify errors channel closed, exiting")
					return
				}

				middleware.Log.Warnw("there was an fsnotify error", "error", err.Error())
			}
		}
	}()
}
