package viewer

import (
	"context"
	"io/ioutil"
	"os"
	"runtime"

	"code.cloudfoundry.org/bytefmt"
	"github.com/dimcz/viewer/internal/config"
	"github.com/dimcz/viewer/pkg/docker"
	"github.com/dimcz/viewer/pkg/logger"
	"github.com/dimcz/viewer/pkg/oviewer"
	"github.com/pkg/errors"
)

type Viewer struct {
	log    *logger.Logger
	cfg    *config.Config
	ctx    context.Context
	cancel func()

	dock  *docker.Docker
	cache *os.File

	ov *oviewer.Root
}

func Init(log *logger.Logger, cfg *config.Config, dock *docker.Docker) (*Viewer, error) {
	ctx, cancel := context.WithCancel(context.Background())

	return &Viewer{
		log:    log,
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
		dock:   dock,
	}, nil
}

func (v *Viewer) Shutdown() {
	v.ov.Close()
	v.cancel()

	if err := os.Remove(v.cache.Name()); err != nil {
		v.log.Error(err)
	}
}

func (v *Viewer) Start() error {
	doc, err := v.newDocument()
	if err != nil {
		return errors.Wrap(err, "failed to create document")
	}

	v.ov, err = oviewer.NewOviewer(doc)
	if err != nil {
		return errors.Wrap(err, "failed to create oviewer")
	}

	v.ov.SetLog(v.log.Debug)
	v.ov.General.FollowMode = true
	v.ov.General.WrapMode = true

	if err := v.ov.SetKeyHandler("prevContainer", []string{"left"}, v.PrevContainer); err != nil {
		return errors.Wrap(err, "failed to bind left key")
	}

	if err := v.ov.SetKeyHandler("nextContainer", []string{"right"}, v.NextContainer); err != nil {
		return errors.Wrap(err, "failed to bind right key")
	}

	if err := v.ov.SetKeyHandler("systemReport", []string{"s"}, v.systemReport); err != nil {
		return errors.Wrap(err, "failed to bind s key")
	}

	if err := v.ov.SetKeyHandler("allLogs", []string{"ctrl+y"}, v.retrieveAllLogs); err != nil {
		return errors.Wrap(err, "failed to bind s key")
	}

	if err := v.ov.Run(); err != nil {
		return errors.Wrap(err, "failed to run oviewer")
	}

	return nil
}

func (v *Viewer) Stop() {
	v.dock.Stop()

	v.log.LogOnErr(v.cache.Close())
	v.log.LogOnErr(os.Remove(v.cache.Name()))
}

func (v *Viewer) NewDocument() error {
	doc, err := v.newDocument()

	if err != nil {
		return errors.Wrap(err, "failed to create document")
	}

	v.ov.ReplaceDocument(doc)

	return nil
}

func (v *Viewer) PrevContainer() {
	v.Stop()

	v.dock.SetPrevContainer()

	if err := v.NewDocument(); err != nil {
		v.log.Fatal(err)
	}
}

func (v *Viewer) NextContainer() {
	v.Stop()

	v.dock.SetNextContainer()

	if err := v.NewDocument(); err != nil {
		v.log.Fatal(err)
	}
}

func (v *Viewer) newDocument() (*oviewer.Document, error) {
	var err error

	v.cache, err = ioutil.TempFile(os.TempDir(), "dlog_")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create temp file")
	}

	v.dock.Load(v.ctx, v.cache, v.cfg.Tail)

	doc, err := oviewer.OpenDocument(v.cache.Name())
	if err != nil {
		return nil, errors.Wrap(err, "failed to open document")
	}

	doc.Caption = v.dock.Name()
	doc.SetLog(v.log.Debug)

	return doc, nil
}

func (v *Viewer) systemReport() {
	var mem runtime.MemStats

	runtime.ReadMemStats(&mem)
	v.log.Debug("systemReport -->")
	v.log.Debug("Total alloc ", bytefmt.ByteSize(mem.TotalAlloc))
	v.log.Debug("Sys ", bytefmt.ByteSize(mem.Sys))
	v.log.Debug("Heap alloc ", bytefmt.ByteSize(mem.HeapAlloc))
	v.log.Debug("Heap sys ", bytefmt.ByteSize(mem.HeapSys))
	v.log.Debug("Goroutines num ", runtime.NumGoroutine())
	v.log.Debug("systemReport <--")
	runtime.GC()
}

func (v *Viewer) retrieveAllLogs() {
	v.Stop()

	var err error

	v.cache, err = ioutil.TempFile(os.TempDir(), "dlog_")
	if err != nil {
		v.log.Fatal(err)
	}

	v.dock.Load(v.ctx, v.cache, 0)

	doc, err := oviewer.OpenDocument(v.cache.Name())
	if err != nil {
		v.log.Fatal(err)
	}

	doc.Caption = v.dock.Name()
	doc.SetLog(v.log.Debug)

	v.ov.ReplaceDocument(doc)
}
