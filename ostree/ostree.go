package ostree

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	glib "github.com/ostreedev/ostree-go/pkg/glibobject"
)

// #cgo pkg-config: ostree-1
// #include <stdlib.h>
// #include <glib.h>
// #include <ostree.h>
import "C"

func openRepo(path string) (*C.struct_OstreeRepo, error) {
	if path == "" {
		return nil, errors.New("empty path")
	}
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	repoPath := C.g_file_new_for_path(cpath)
	defer C.g_object_unref(C.gpointer(repoPath))
	repo := C.ostree_repo_new(repoPath)

	var cerr *C.GError
	r := glib.GoBool(glib.GBoolean(C.ostree_repo_open(repo, nil, &cerr)))
	if !r {
		return nil, generateError(cerr)
	}
	return repo, nil
}

func ListRefs(path string) ([]string, error) {
	var gerr = glib.NewGError()
	var cerr = (*C.GError)(gerr.Ptr())
	defer C.free(unsafe.Pointer(cerr))

	repo, err := openRepo(path)
	if err != nil {
		return nil, err
	}

	var rawRefs unsafe.Pointer
	if !glib.GoBool(glib.GBoolean(C.ostree_repo_list_refs(repo, nil, (**C.GHashTable)((unsafe.Pointer)(&rawRefs)), nil, &cerr))) {
		return nil, generateError(cerr)
	}

	refs := (*C.GHashTable)(rawRefs)
	var hashIter C.GHashTableIter
	var hashkey, hashvalue C.gpointer
	var ret []string

	C.g_hash_table_iter_init(&hashIter, refs)
	for glib.GoBool(glib.GBoolean(C.g_hash_table_iter_next(&hashIter, &hashkey, &hashvalue))) {
		ret = append(ret, C.GoString((*C.char)(hashkey)))
	}
	return ret, nil
}

func generateError(err *C.GError) error {
	if err == nil {
		return errors.New("nil GError")
	}

	goErr := glib.ConvertGError(glib.ToGError(unsafe.Pointer(err)))
	_, file, line, ok := runtime.Caller(1)
	if ok {
		return fmt.Errorf("%s:%d - %s", file, line, goErr)
	}
	return goErr
}
