// 生成用于联调的合成 GeoIP 数据库（GEOIP: 203.0.113.0/24=US/California，
// 198.51.100.0/24=CN/Shanghai）。
package main

import (
	"log"
	"net"
	"os"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
)

func city(country, region, name string) mmdbtype.Map {
	return mmdbtype.Map{
		"country": mmdbtype.Map{"iso_code": mmdbtype.String(country)},
		"subdivisions": mmdbtype.Slice{
			mmdbtype.Map{"names": mmdbtype.Map{"en": mmdbtype.String(region)}},
		},
		"city": mmdbtype.Map{"names": mmdbtype.Map{"en": mmdbtype.String(name)}},
	}
}

func main() {
	w, err := mmdbwriter.New(mmdbwriter.Options{
		DatabaseType: "GeoLite2-City", RecordSize: 24, IPVersion: 6,
		IncludeReservedNetworks: true,
	})
	if err != nil {
		log.Fatal(err)
	}
	for cidr, rec := range map[string]mmdbtype.Map{
		"203.0.113.0/24": city("US", "California", "Los Angeles"),
		"198.51.100.0/24": city("CN", "Shanghai", "Shanghai"),
		"127.0.0.0/8":    city("CN", "Beijing", "Beijing"),
	} {
		_, network, _ := net.ParseCIDR(cidr)
		if err := w.Insert(network, rec); err != nil {
			log.Fatal(err)
		}
	}
	f, err := os.Create("/tmp/kamc-geoip/GeoLite2-City-Test.mmdb")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if _, err := w.WriteTo(f); err != nil {
		log.Fatal(err)
	}
	log.Println("geoip test db written")
}
