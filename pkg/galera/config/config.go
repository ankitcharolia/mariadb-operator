package config

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"text/template"

	mariadbv1alpha1 "github.com/mariadb-operator/mariadb-operator/api/v1alpha1"
	"github.com/mariadb-operator/mariadb-operator/pkg/environment"
	"github.com/mariadb-operator/mariadb-operator/pkg/galera/recovery"
	"github.com/mariadb-operator/mariadb-operator/pkg/statefulset"
	"k8s.io/utils/ptr"
)

const (
	ConfigFileName    = "0-galera.cnf"
	BootstrapFileName = recovery.BootstrapFileName
)

var BootstrapFile = []byte(`[galera]
wsrep_new_cluster="ON"`)

type ConfigFile struct {
	mariadb *mariadbv1alpha1.MariaDB
}

func NewConfigFile(mariadb *mariadbv1alpha1.MariaDB) *ConfigFile {
	return &ConfigFile{
		mariadb: mariadb,
	}
}

func (c *ConfigFile) Marshal(podEnv *environment.PodEnvironment) ([]byte, error) {
	if !c.mariadb.IsGaleraEnabled() {
		return nil, errors.New("MariaDB Galera not enabled, unable to render config file")
	}
	galera := ptr.Deref(c.mariadb.Spec.Galera, mariadbv1alpha1.Galera{})

	tpl := createTpl("galera", `[mariadb]
bind-address=0.0.0.0
default_storage_engine=InnoDB
binlog_format=row
innodb_autoinc_lock_mode=2

# Cluster configuration
wsrep_on=ON
wsrep_provider={{ .GaleraLibPath }}
wsrep_cluster_address="{{ .ClusterAddress }}"
wsrep_cluster_name=mariadb-operator
wsrep_slave_threads={{ .Threads }}

# Node configuration
wsrep_node_address="{{ .NodeAddress }}"
wsrep_node_name="{{ .Pod }}"
wsrep_sst_method="{{ .SST }}"
{{- if .SSTAuth }}
wsrep_sst_auth="root:{{ .RootPassword }}"
{{- end }}
`)
	buf := new(bytes.Buffer)
	clusterAddr, err := c.clusterAddress()
	if err != nil {
		return nil, fmt.Errorf("error getting cluster address: %v", err)
	}
	sst, err := galera.SST.MariaDBFormat()
	if err != nil {
		return nil, fmt.Errorf("error getting SST: %v", err)
	}

	err = tpl.Execute(buf, struct {
		ClusterAddress string
		NodeAddress    string
		GaleraLibPath  string
		Threads        int
		Pod            string
		SST            string
		SSTAuth        bool
		RootPassword   string
	}{
		ClusterAddress: clusterAddr,
		NodeAddress:    podEnv.PodIP,
		GaleraLibPath:  galera.GaleraLibPath,
		Threads:        galera.ReplicaThreads,
		Pod:            podEnv.PodName,
		SST:            sst,
		SSTAuth:        galera.SST == mariadbv1alpha1.SSTMariaBackup || galera.SST == mariadbv1alpha1.SSTMysqldump,
		RootPassword:   podEnv.MariadbRootPassword,
	})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (c *ConfigFile) clusterAddress() (string, error) {
	if c.mariadb.Spec.Replicas == 0 {
		return "", errors.New("at least one replica must be specified to get a valid cluster address")
	}
	pods := make([]string, c.mariadb.Spec.Replicas)
	for i := 0; i < int(c.mariadb.Spec.Replicas); i++ {
		pods[i] = statefulset.PodFQDNWithService(
			c.mariadb.ObjectMeta,
			i,
			c.mariadb.InternalServiceKey().Name,
		)
	}
	return fmt.Sprintf("gcomm://%s", strings.Join(pods, ",")), nil
}

func createTpl(name, t string) *template.Template {
	return template.Must(template.New(name).Parse(t))
}
