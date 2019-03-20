package chinese_address_code

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"strings"

	"regexp"

	"errors"

	"golang.org/x/net/html"
	"golang.org/x/text/encoding/simplifiedchinese"
)

type Location struct {
	Province string
	City     string
	County   string
	Town     string
	Village  string
}

var CacheFile = ".chinese_location_code.json"

type ProvinceMap struct {
	Basic
	Cities map[string]*City
}

type City struct {
	Basic
	Distincts map[string]*DistinctMap
}

type DistinctMap struct {
	Basic
	Streets map[string]*StreetMap
}

type StreetMap struct {
	Basic
	Distincts map[string]Location
}

var cacheMap map[string]*ProvinceMap

var SearchUrl = "http://www.stats.gov.cn/tjsj/tjbz/tjyqhdmhcxhfdm/2015/"

func read() (map[string]*ProvinceMap, error) {
	var m = make(map[string]*ProvinceMap)
	var bts []byte
	var err error
	if bts, err = ioutil.ReadFile(CacheFile); err == nil {
		err = json.Unmarshal(bts, &m)
	}
	return m, nil
}

//Search 提供地区编码，返回标准地址
func Search(code string) (*Location, error) {
	changed := false
	defer func() {
		if changed {
			write(cacheMap)
		}
	}()

	var ProvinceCode, sTypeCode, distinctCode = "00", "00", "00"

	if len(code) < 2 {
		return nil, errors.New("地区编码位数不足")
	} else {
		ProvinceCode = code[:2]
		if len(code) >= 4 {
			sTypeCode = code[2:4]
		}
		if len(code) >= 6 {
			distinctCode = code[4:6]
		}
	}

	if sTypeCode == "00" {
		sTypeCode = "01"
	}

	if distinctCode == "00" {
		distinctCode = "01"
	}
	// streetCode := code[6:9]
	// blockCode := code[9:12]
	var err error
	if cacheMap == nil {
		if cacheMap, err = read(); err != nil {
			return nil, err
		}
	}

	var p *ProvinceMap
	ok := false
	p, ok = cacheMap[ProvinceCode]

	if !ok {
		if err = searchProvince(ProvinceCode, cacheMap); err != nil {
			return nil, err
		}
		changed = true
	}
	if p, ok = cacheMap[ProvinceCode]; ok {
		ok = false
		var city *City
		if p.Cities == nil {
			p.Cities = make(map[string]*City)
		}

		city, ok = p.Cities[ProvinceCode+sTypeCode]
		if !ok {
			searchCity(ProvinceCode, ProvinceCode+sTypeCode, p.Cities)
			changed = true
		}
		if city, ok = p.Cities[ProvinceCode+sTypeCode]; ok {
			ok = false
			var distinct *DistinctMap
			if city.Distincts == nil {
				city.Distincts = make(map[string]*DistinctMap)
			}
			distinct, ok = city.Distincts[ProvinceCode+sTypeCode+distinctCode]
			if !ok {
				searchDistinct(ProvinceCode+"/"+ProvinceCode+sTypeCode, ProvinceCode+sTypeCode+distinctCode, city.Distincts)
				changed = true
			}
			if distinct, ok = city.Distincts[ProvinceCode+sTypeCode+distinctCode]; ok {

				return &Location{
					Province: p.Name,
					City:     city.Name,
					County:   distinct.Name,
				}, nil
			} else {
				err = fmt.Errorf("区县代码(%s)不能找到", distinctCode)
			}
		} else {
			err = fmt.Errorf("市代码(%s)不能找到", sTypeCode)
		}
	} else {
		err = fmt.Errorf("省代码(%s)不能找到", sTypeCode)
	}
	return nil, err
}

func searchProvince(pCode string, parent map[string]*ProvinceMap) error {
	return search("", pCode, func() Node {
		return new(ProvinceMap)
	}, func(code string, data interface{}) {
		if p, ok := data.(*ProvinceMap); ok {
			parent[code] = p
		}
	})
}

func searchCity(pCode string, cityCode string, parent map[string]*City) error {
	return searchDetail(pCode, cityCode, func() Node {
		return new(City)
	}, func(code string, data interface{}) {
		if p, ok := data.(*City); ok {
			parent[code[0:4]] = p
		}
	})
}

func searchDistinct(pCode string, subCode string, parent map[string]*DistinctMap) error {
	return searchDetail(pCode, subCode, func() Node {
		return new(DistinctMap)
	}, func(code string, data interface{}) {
		if p, ok := data.(*DistinctMap); ok {
			fmt.Println(code)
			parent[code[0:6]] = p
		}
	})
}

func searchStreet(pCode string, subCode string, parent map[string]*StreetMap) error {
	return searchDetail(pCode, subCode, func() Node {
		return new(StreetMap)
	}, func(code string, data interface{}) {
		if p, ok := data.(*StreetMap); ok {
			parent[code[6:9]] = p
		}
	})
}

func write(m map[string]*ProvinceMap) {
	bts, _ := json.Marshal(m)
	ioutil.WriteFile(CacheFile, bts, 0666)
}

func search(suffix string, pCode string, newNode func() Node, addToParent func(string, interface{})) (err error) {
	var f func(*html.Node) error
	f = func(n *html.Node) error {
		if n.Type == html.ElementNode && n.Data == "a" {
			var p = newNode()
			var code string
			var found bool
			for _, a := range n.Attr {
				if a.Key == "href" {
					strs := strings.Split(a.Val, ".")
					if len(strs) > 0 {
						if ok, err := regexp.MatchString("[0-9]+", strs[0]); err == nil && ok {
							if suffix != "" {
								strs := strings.Split(strs[0], "/")
								code = strs[len(strs)-1]
							} else {
								code = strs[0]
							}
							found = true
							break
						} else if err != nil {
							return err
						}
					}
				}
			}
			if found {
				if name, err := simplifiedchinese.GBK.NewDecoder().String(n.FirstChild.Data); err == nil {
					p.SetName(name)
					addToParent(code, p)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if err := f(c); err != nil {
				return err
			}
		}
		return nil
	}
	var res *http.Response
	url := SearchUrl
	if suffix != "" {
		url += suffix + ".html"
	}
	if res, err = http.Get(url); err == nil && res.StatusCode < 300 {
		var doc *html.Node
		if doc, err = html.Parse(res.Body); err == nil {
			if err = f(doc); err != nil {
				return
			}
		}
	} else if err == nil {
		err = errors.New(res.Status)
	}
	return
}

func searchDetail(suffix string, pCode string, newNode func() Node, addToParent func(string, interface{})) (err error) {
	var f func(*html.Node) error
	f = func(n *html.Node) error {
		if n.Type == html.ElementNode && n.Data == "tr" {
			var found bool
			for _, a := range n.Attr {
				if a.Key == "class" && (a.Val == "citytr" || a.Val == "countytr" || a.Val == "towntr" || a.Val == "villagetr") {
					found = true
				}
			}

			if found {
				var p = newNode()
				var code, gbkName string
				if n.FirstChild.FirstChild.Data == "a" {
					code = n.FirstChild.FirstChild.FirstChild.Data
					gbkName = n.LastChild.FirstChild.FirstChild.Data
				} else {
					code = n.FirstChild.FirstChild.Data
					gbkName = n.LastChild.FirstChild.Data
				}

				if name, err := simplifiedchinese.GBK.NewDecoder().String(gbkName); err == nil {
					p.SetName(name)
					addToParent(code, p)
					return err
				}
				return nil
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if err := f(c); err != nil {
				return err
			}
		}
		return nil
	}
	var res *http.Response
	url := SearchUrl
	if suffix != "" {
		url += suffix + ".html"
	}
	if res, err = http.Get(url); err == nil && res.StatusCode < 300 {
		var doc *html.Node
		if doc, err = html.Parse(res.Body); err == nil {
			if err = f(doc); err != nil {
				return
			}
		}
	} else if err == nil {
		err = errors.New(res.Status)
	}
	return
}
