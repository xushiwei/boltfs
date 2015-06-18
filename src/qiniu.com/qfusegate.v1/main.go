package qfusegate

import (
	"encoding/json"
	"io"
	"os"
	"sync"

	"bazil.org/fuse"
	"github.com/qiniu/log.v1"
)

// ---------------------------------------------------------------------------

type Config struct {
	SaveToFile string `json:"save_to"`
	BackupFile string `json:"backup_to"`
}

type Service struct {
	Config

	mounts []*MountArgs
	conns  map[string]*Conn // mountpoint => conn
	mutex  sync.Mutex
}

func New(cfg *Config) (p *Service, err error) {

	var mounts []*MountArgs

	f, err := os.Open(cfg.SaveToFile)
	if err == nil {
		json.NewDecoder(f).Decode(&mounts)
		f.Close()
	}

	p = &Service{
		Config: *cfg,
	}
	for _, args := range mounts {
		fuse.Unmount(args.MountPoint)
		err = p.mount(args)
		if err != nil {
			return
		}
	}
	return
}

// ---------------------------------------------------------------------------

type MountArgs struct {
	// 加载点
	//
	MountPoint  string `json:"mountpoint"`

	// 目标文件系统位置(Host)。如 "http://127.0.0.1:7777"
	//
	TargeFSHost string `json:"target"`

	// the file system name (also called source) that is visible in the list of mounted file systems
	//
	FSName      string `json:"fsname"`

	// Subtype sets the subtype of the mount. The main type is always `fuse`.
	// The type in a list of mounted file systems will look like `fuse.foo`.
	//
	Subtype     string `json:"subtype"`

	// VolumeName sets the volume name shown in Finder.
	// OS X only. Others ignore this option.
	//
	Name        string `json:"name"`

	// "allow_other" allows other users to access the file system.
	// "allow_root" allows other users to access the file system.
	//
	AllowMode   string `json:"allow"`

	// ReadOnly makes the mount read-only.
	//
	ReadOnly    int    `json:"readonly"`
}

func (p *Service) PostMount(args *MountArgs) (err error) {

	p.mutex.Lock()
	err = p.mount(args)
	if err == nil {
		p.mounts = append(p.mounts, args)
		err = p.save()
	}
	p.mutex.Unlock()
	return
}

func (p *Service) save() (err error) {

	os.Remove(p.BackupFile)

	backup, err := os.Create(p.BackupFile)
	if err != nil {
		return
	}
	defer backup.Close()

	old, err := os.Open(p.SaveToFile)
	if err == nil {
		io.Copy(backup, old)
		old.Close()
	}

	f, err := os.Create(p.SaveToFile)
	if err != nil {
		return
	}
	defer f.Close()

	err = json.NewEncoder(f).Encode(p.mounts)
	return
}

func (p *Service) mount(args *MountArgs) (err error) {

	options, err := mountOptions(args)
	if err != nil {
		return
	}

	c, err := fuse.Mount(args.MountPoint, options...)
	if err != nil {
		return
	}

	conn, err := NewConn(c, args)
	if err != nil {
		return
	}
	p.conns[args.MountPoint] = conn

	go func() {
		err := conn.Serve()
		if err != nil {
			log.Error("Serve failed:", err, "mount:", *args)
		}
	}()
	return
}

func mountOptions(args *MountArgs) (options []fuse.MountOption, err error) {

	return
}

// ---------------------------------------------------------------------------

type unmountArgs struct {
	MountPoint string `json:"mountpoint"`
}

func (p *Service) PostUnmount(args *unmountArgs) (err error) {

	return fuse.Unmount(args.MountPoint)
}

// ---------------------------------------------------------------------------
