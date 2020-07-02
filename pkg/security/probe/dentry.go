package probe

import (
	"C"

	"bytes"
	"encoding/binary"
	"fmt"
	"unsafe"

	"github.com/pkg/errors"

	eprobe "github.com/DataDog/datadog-agent/pkg/ebpf/probe"
	"github.com/hashicorp/golang-lru/simplelru"
)

type DentryResolver struct {
	probe     *eprobe.Probe
	pathnames eprobe.Table
	lru       *simplelru.LRU
}

type PathKey struct {
	inode   uint64
	mountID uint32
}

func (p *PathKey) Write(buffer []byte) {
	byteOrder.PutUint64(buffer[0:8], p.inode)
	byteOrder.PutUint32(buffer[8:12], p.mountID)
	byteOrder.PutUint32(buffer[12:16], 0)
}

func (p *PathKey) Read(buffer []byte) {
	p.inode = byteOrder.Uint64(buffer[0:8])
	p.mountID = byteOrder.Uint32(buffer[8:12])
}

func (p *PathKey) IsNull() bool {
	return p.inode == 0 && p.mountID == 0
}

func (p *PathKey) String() string {
	return fmt.Sprintf("%x/%x", p.mountID, p.inode)
}

type PathValue struct {
	parent PathKey
	name   [256]byte
}

func (dr *DentryResolver) getName(mountID uint32, inode uint64) (name string, err error) {
	key := PathKey{mountID: mountID, inode: inode}
	if key.IsNull() {
		return "", fmt.Errorf("invalid inode/mountID couple: %s", key.String())
	}

	keyBuffer := make([]byte, 16)
	key.Write(keyBuffer)
	pathRaw := []byte{}
	var nameRaw [256]byte

	if pathRaw, err = dr.pathnames.Get(keyBuffer); err != nil {
		return "", fmt.Errorf("unable to get filename for mountID `%d` and inode `%d`", mountID, inode)
	}

	if err = binary.Read(bytes.NewBuffer(pathRaw[16:]), byteOrder, &nameRaw); err != nil {
		return "", errors.Wrap(err, "failed to decode received data (pathLeaf)")
	}

	return C.GoString((*C.char)(unsafe.Pointer(&nameRaw))), nil
}

func (dr *DentryResolver) GetName(mountID uint32, inode uint64) string {
	name, _ := dr.getName(mountID, inode)
	return name
}

// Resolve the pathname of a dentry, starting at the pathnameKey in the pathnames table
func (dr *DentryResolver) resolve(mountID uint32, inode uint64) (filename string, err error) {
	// Don't resolve path if pathnameKey isn't valid
	key := PathKey{mountID: mountID, inode: inode}
	if key.IsNull() {
		return "", fmt.Errorf("invalid inode/mountID couple: %s", key.String())
	}

	keyBuffer := make([]byte, 16)
	key.Write(keyBuffer)
	done := false
	pathRaw := []byte{}
	var path PathValue

	// Fetch path recursively
	for !done {
		if value, found := dr.lru.Get(string(keyBuffer)); !found {
			if pathRaw, err = dr.pathnames.Get(keyBuffer); err != nil {
				filename = "*ERROR*" + filename
				break
			}

			path.parent.Read(pathRaw)
			if err = binary.Read(bytes.NewBuffer(pathRaw[16:]), byteOrder, &path.name); err != nil {
				err = errors.Wrap(err, "failed to decode received data (pathLeaf)")
				break
			}

			dr.lru.Add(string(keyBuffer), path)
		} else {
			path = value.(PathValue)
		}

		// Don't append dentry name if this is the root dentry (i.d. name == '/')
		if path.name[0] != '\x00' && path.name[0] != '/' {
			filename = "/" + C.GoString((*C.char)(unsafe.Pointer(&path.name))) + filename
		}

		if path.parent.inode == 0 {
			break
		}

		// Prepare next key
		path.parent.Write(keyBuffer)
	}

	if len(filename) == 0 {
		filename = "/"
	}

	return
}

// Resolve the pathname of a dentry, starting at the pathnameKey in the pathnames table
func (dr *DentryResolver) Resolve(mountID uint32, inode uint64) string {
	path, _ := dr.resolve(mountID, inode)
	return path
}

func (dr *DentryResolver) Start() error {
	pathnames := dr.probe.Table("pathnames")
	if pathnames == nil {
		return fmt.Errorf("pathnames BPF_HASH table doesn't exist")
	}
	dr.pathnames = pathnames

	return nil
}

func NewDentryResolver(probe *eprobe.Probe) (*DentryResolver, error) {
	lru, err := simplelru.NewLRU(1024, nil)
	if err != nil {
		return nil, err
	}

	return &DentryResolver{
		lru:   lru,
		probe: probe,
	}, nil
}