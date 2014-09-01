package bh

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/coreos/go-etcd/etcd"
	"github.com/golang/glog"
)

const (
	regPrefix    = "beehive"
	regAppDir    = "apps"
	regHiveDir   = "hives"
	regTtl       = 0
	expireAction = "expire"
	lockFileName = "__lock__"
)

type registery struct {
	*etcd.Client
	prefix  string
	hiveDir string
	appDir  string
	ttl     uint64
}

func (h *hive) connectToRegistery() {
	if len(h.config.RegAddrs) == 0 {
		return
	}

	// TODO(soheil): Add TLS registery.
	h.registery = registery{
		Client:  etcd.NewClient(h.config.RegAddrs),
		prefix:  regPrefix,
		hiveDir: regHiveDir,
		appDir:  regAppDir,
		ttl:     regTtl,
	}
	if ok := h.registery.SyncCluster(); !ok {
		glog.Fatalf("Cannot connect to registery nodes: %s", h.config.RegAddrs)
	}
}

func (g registery) connected() bool {
	return g.Client == nil
}

type hiveRegVal HiveId

func (g *registery) registerHive(h *hive) {

}

type beeRegVal struct {
	HiveId HiveId `json:"hive_id"`
	BeeId  uint32 `json:"bee_id"`
}

func (this *beeRegVal) Eq(that *beeRegVal) bool {
	return this.HiveId == that.HiveId && this.BeeId == that.BeeId
}

func unmarshallRegVal(d string) (beeRegVal, error) {
	var v beeRegVal
	err := json.Unmarshal([]byte(d), &v)
	return v, err
}

func unmarshallRegValOrFail(d string) beeRegVal {
	v, err := unmarshallRegVal(d)
	if err != nil {
		glog.Fatalf("Cannot unmarshall registery value %v: %v", d, err)
	}
	return v
}

func marshallRegVal(v beeRegVal) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

func marshallRegValOrFail(v beeRegVal) string {
	d, err := marshallRegVal(v)
	if err != nil {
		glog.Fatalf("Cannot marshall registery value %v: %v", v, err)
	}
	return d
}

func (g registery) path(elem ...string) string {
	return g.prefix + "/" + strings.Join(elem, "/")
}

func (g registery) appPath(elem ...string) string {
	return g.prefix + "/" + g.appDir + "/" + strings.Join(elem, "/")
}

func (g registery) hivePath(elem ...string) string {
	return g.prefix + "/" + g.hiveDir + "/" + strings.Join(elem, "/")
}

func (g registery) lockApp(id BeeId) error {
	// TODO(soheil): For lock and unlock we can use etcd indices but
	// v.Temp might be changed by the app. Check this and fix it if possible.
	v := beeRegVal{
		HiveId: id.HiveId,
		BeeId:  id.Id,
	}
	k := g.appPath(string(id.AppName), lockFileName)

	for {
		_, err := g.Create(k, marshallRegValOrFail(v), g.ttl)
		if err == nil {
			return nil
		}

		_, err = g.Watch(k, 0, false, nil, nil)
		if err != nil {
			return err
		}
	}
}

func (g registery) unlockApp(id BeeId) error {
	v := beeRegVal{
		HiveId: id.HiveId,
		BeeId:  id.Id,
	}
	k := g.appPath(string(id.AppName), lockFileName)

	res, err := g.Get(k, false, false)
	if err != nil {
		return err
	}

	tempV := unmarshallRegValOrFail(res.Node.Value)
	if !v.Eq(&tempV) {
		return errors.New(
			fmt.Sprintf("Unlocking someone else's lock: %v, %v", v, tempV))
	}

	_, err = g.Delete(k, false)
	if err != nil {
		return err
	}

	return nil
}

func (g registery) set(id BeeId, ms MapSet) beeRegVal {
	err := g.lockApp(id)
	if err != nil {
		glog.Fatalf("Cannot lock app %v: %v", id, err)
	}

	defer func() {
		err := g.unlockApp(id)
		if err != nil {
			glog.Fatalf("Cannot unlock app %v: %v", id, err)
		}
	}()

	sort.Sort(ms)

	v := beeRegVal{
		HiveId: id.HiveId,
		BeeId:  id.Id,
	}
	mv := marshallRegValOrFail(v)
	for _, dk := range ms {
		k := g.appPath(string(id.AppName), string(dk.Dict), string(dk.Key))
		_, err := g.Set(k, mv, g.ttl)
		if err != nil {
			glog.Fatalf("Cannot set bee: %+v", k)
		}
	}
	return v
}

func (g registery) storeOrGet(id BeeId, ms MapSet) beeRegVal {
	err := g.lockApp(id)
	if err != nil {
		glog.Fatalf("Cannot lock app %v: %v", id, err)
	}

	defer func() {
		err := g.unlockApp(id)
		if err != nil {
			glog.Fatalf("Cannot unlock app %v: %v", id, err)
		}
	}()

	sort.Sort(ms)

	v := beeRegVal{
		HiveId: id.HiveId,
		BeeId:  id.Id,
	}
	mv := marshallRegValOrFail(v)
	validate := false
	for _, dk := range ms {
		k := g.appPath(string(id.AppName), string(dk.Dict), string(dk.Key))
		res, err := g.Get(k, false, false)
		if err != nil {
			continue
		}

		resV := unmarshallRegValOrFail(res.Node.Value)
		if resV.Eq(&v) {
			continue
		}

		if validate {
			glog.Fatalf("Incosistencies for bee %v: %v, %v", id, v, resV)
		}

		v = resV
		mv = res.Node.Value
		validate = true
	}

	for _, dk := range ms {
		k := g.appPath(string(id.AppName), string(dk.Dict), string(dk.Key))
		g.Create(k, mv, g.ttl)
	}

	return v
}
