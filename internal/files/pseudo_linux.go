//go:build linux

package files

import "golang.org/x/sys/unix"

// Pseudo/virtual filesystem magic numbers (linux/magic.h). A directory on
// one of these is kernel state, not user data: walking it in a zip stream
// is at best noise and at worst unbounded (/proc/kcore reports itself as a
// regular file of enormous size).
var pseudoFSMagic = map[int64]bool{
	0x9fa0:     true, // proc
	0x62656572: true, // sysfs
	0x1cd1:     true, // devpts
	0x01021994: true, // tmpfs / devtmpfs (incl. /run, /dev/shm)
	0x27e0eb:   true, // cgroup
	0x63677270: true, // cgroup2
	0x64626720: true, // debugfs
	0x74726163: true, // tracefs
	0x73636673: true, // securityfs
	0x6165676c: true, // pstore
	0xde5e81e4: true, // efivarfs
	0xcafe4a11: true, // bpf
	0x62656570: true, // configfs
	0x65735543: true, // fusectl
	0x19800202: true, // mqueue
	0x42494e4d: true, // binfmt_misc
	0x958458f6: true, // hugetlbfs
	0x6e736673: true, // nsfs
	0x0187:     true, // autofs
	0xf97cff8c: true, // selinuxfs
	0x67596969: true, // rpc_pipefs
}

// isPseudoFS reports whether path lives on a kernel pseudo-filesystem.
// Errors count as "not pseudo": a stat failure on real data should surface
// from the actual read, not silently skip the tree.
func isPseudoFS(path string) bool {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return false
	}
	return pseudoFSMagic[int64(st.Type)]
}
