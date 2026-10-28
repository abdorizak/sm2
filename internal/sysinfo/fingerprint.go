package sysinfo

import (
	"path/filepath"
	"strings"
)

// product is a piece of server software sm2 recognizes by name.
type product struct {
	id, name, kind string
	// match lists names (unit, binary, or image) that identify the product. A
	// name matches exactly or as a prefix followed by a non-letter
	// ("redis" ~ "redis-server", "postgres" ~ "postgres16"); a leading "*"
	// matches anywhere ("*fpm" ~ "php8.3-fpm").
	match []string
}

// products is checked in order; put more specific names first.
var products = []product{
	{"postgres", "PostgreSQL", "database", []string{"postgres", "postgresql", "postmaster"}},
	{"mariadb", "MariaDB", "database", []string{"mariadb", "mariadbd"}},
	{"mysql", "MySQL", "database", []string{"mysql", "mysqld"}},
	{"mongodb", "MongoDB", "database", []string{"mongod", "mongodb", "mongo"}},
	{"clickhouse", "ClickHouse", "database", []string{"clickhouse", "clickhouse-server"}},
	{"cockroach", "CockroachDB", "database", []string{"cockroach"}},
	{"cassandra", "Cassandra", "database", []string{"cassandra"}},
	{"influxdb", "InfluxDB", "database", []string{"influxd", "influxdb"}},
	{"couchdb", "CouchDB", "database", []string{"couchdb"}},
	{"neo4j", "Neo4j", "database", []string{"neo4j"}},
	{"etcd", "etcd", "database", []string{"etcd"}},

	{"redis", "Redis", "cache", []string{"redis", "redis-server"}},
	{"valkey", "Valkey", "cache", []string{"valkey", "valkey-server"}},
	{"keydb", "KeyDB", "cache", []string{"keydb", "keydb-server"}},
	{"memcached", "Memcached", "cache", []string{"memcached"}},

	{"rabbitmq", "RabbitMQ", "queue", []string{"rabbitmq", "rabbitmq-server"}},
	{"kafka", "Kafka", "queue", []string{"kafka"}},
	{"nats", "NATS", "queue", []string{"nats", "nats-server"}},
	{"mosquitto", "Mosquitto (MQTT)", "queue", []string{"mosquitto"}},

	{"elasticsearch", "Elasticsearch", "search", []string{"elasticsearch"}},
	{"opensearch", "OpenSearch", "search", []string{"opensearch"}},
	{"meilisearch", "Meilisearch", "search", []string{"meilisearch"}},
	{"typesense", "Typesense", "search", []string{"typesense", "typesense-server"}},

	{"nginx", "nginx", "web", []string{"nginx"}},
	{"apache", "Apache httpd", "web", []string{"apache2", "httpd"}},
	{"caddy", "Caddy", "web", []string{"caddy"}},
	{"traefik", "Traefik", "web", []string{"traefik"}},
	{"haproxy", "HAProxy", "web", []string{"haproxy"}},
	{"php-fpm", "PHP-FPM", "web", []string{"*fpm"}},

	{"node_exporter", "Node Exporter", "monitoring", []string{"node_exporter", "prometheus-node-exporter"}},
	{"prometheus", "Prometheus", "monitoring", []string{"prometheus"}},
	{"grafana", "Grafana", "monitoring", []string{"grafana", "grafana-server"}},

	{"postfix", "Postfix", "mail", []string{"postfix"}},
	{"exim", "Exim", "mail", []string{"exim", "exim4"}},
	{"dovecot", "Dovecot", "mail", []string{"dovecot"}},

	{"systemd-resolved", "systemd-resolved", "dns", []string{"systemd-resolved"}},
	{"bind", "BIND", "dns", []string{"named", "bind9"}},
	{"unbound", "Unbound", "dns", []string{"unbound"}},
	{"dnsmasq", "dnsmasq", "dns", []string{"dnsmasq"}},

	{"ssh", "OpenSSH", "remote", []string{"ssh", "sshd"}},
	{"tailscale", "Tailscale", "remote", []string{"tailscaled", "tailscale"}},
	{"wireguard", "WireGuard", "remote", []string{"wg-quick"}},
	{"openvpn", "OpenVPN", "remote", []string{"openvpn"}},

	{"fail2ban", "fail2ban", "security", []string{"fail2ban", "fail2ban-server"}},
	{"crowdsec", "CrowdSec", "security", []string{"crowdsec"}},

	{"docker", "Docker Engine", "runtime", []string{"docker", "dockerd"}},
	{"containerd", "containerd", "runtime", []string{"containerd"}},
	{"podman", "Podman", "runtime", []string{"podman"}},
	{"k3s", "k3s", "runtime", []string{"k3s"}},
	{"kubelet", "kubelet", "runtime", []string{"kubelet"}},
}

// portHints maps well-known ports to the products that normally own them.
// Used only for sockets whose owning process we can't see.
var portHints = map[int][]string{
	22:    {"ssh"},
	25:    {"postfix", "exim"},
	53:    {"systemd-resolved", "bind", "unbound", "dnsmasq"},
	80:    {"nginx", "apache", "caddy", "traefik", "haproxy"},
	443:   {"nginx", "apache", "caddy", "traefik", "haproxy"},
	465:   {"postfix", "exim"},
	587:   {"postfix", "exim"},
	993:   {"dovecot"},
	1883:  {"mosquitto"},
	2379:  {"etcd"},
	3306:  {"mysql", "mariadb"},
	4222:  {"nats"},
	5432:  {"postgres"},
	5672:  {"rabbitmq"},
	5984:  {"couchdb"},
	6379:  {"redis", "valkey", "keydb"},
	7687:  {"neo4j"},
	7700:  {"meilisearch"},
	8086:  {"influxdb"},
	8108:  {"typesense"},
	8123:  {"clickhouse"},
	9042:  {"cassandra"},
	9090:  {"prometheus"},
	9092:  {"kafka"},
	9100:  {"node_exporter"},
	9200:  {"elasticsearch", "opensearch"},
	11211: {"memcached"},
	15672: {"rabbitmq"},
	26257: {"cockroach"},
	27017: {"mongodb"},
}

func productByID(id string) *product {
	for i := range products {
		if products[i].id == id {
			return &products[i]
		}
	}
	return nil
}

// identify returns the first product any of the names match.
func identify(names ...string) *product {
	for i := range products {
		p := &products[i]
		for _, n := range names {
			n = strings.ToLower(n)
			if n == "" {
				continue
			}
			for _, m := range p.match {
				if matchName(n, m) {
					return p
				}
			}
		}
	}
	return nil
}

func matchName(name, m string) bool {
	if strings.HasPrefix(m, "*") {
		return strings.Contains(name, m[1:])
	}
	if !strings.HasPrefix(name, m) {
		return false
	}
	if len(name) == len(m) {
		return true
	}
	c := name[len(m)]
	return c < 'a' || c > 'z'
}

// fingerprint sets a service's product and kind from whatever names it has:
// unit, binary, image, or the commands in its process group.
func fingerprint(s *Service, procs map[int]*proc) {
	var names []string
	switch s.Source {
	case SourceDocker:
		names = append(names, imageName(s.Image))
	case SourceSystemd:
		unit := strings.TrimSuffix(s.Unit, ".service")
		if i := strings.IndexByte(unit, '@'); i >= 0 {
			names = append(names, unit[:i], unit)
		} else {
			names = append(names, unit)
		}
		names = append(names, filepath.Base(s.Exe))
		if p := procs[s.PID]; p != nil {
			names = append(names, procName(p))
		}
	default: // sm2 apps and unmanaged processes: look at the whole group
		if p := procs[s.PID]; p != nil {
			names = append(names, procName(p), filepath.Base(firstWord(p.args)))
		}
		for _, p := range procs {
			if s.PID > 0 && p.pgid == s.PID && p.pid != s.PID {
				names = append(names, procName(p))
			}
		}
	}
	if p := identify(names...); p != nil {
		s.productID, s.Product = p.id, p.name
		if s.Kind != "listener" {
			s.Kind = p.kind
		}
	}
}
