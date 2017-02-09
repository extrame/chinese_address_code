package chinese_location_code

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
	Distinct string
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

func read() map[string]*ProvinceMap {
	var m = make(map[string]*ProvinceMap)
	if bts, err := ioutil.ReadFile(CacheFile); err == nil {
		json.Unmarshal(bts, m)
	}
	return m
}

//Search 提供地区编码，返回标准地址
func Search(code string) (*Location, error) {
	if len(code) < 6 {
		return nil, errors.New("地区编码位数不足")
	}
	ProvinceCode := code[:2]
	sTypeCode := code[2:4]
	distinctCode := code[4:6]
	// streetCode := code[6:9]
	// blockCode := code[9:12]
	if cacheMap == nil {
		cacheMap = read()
	}

	var p *ProvinceMap
	var err error
	ok := false
	p, ok = cacheMap[ProvinceCode]
	changed := false
	if !ok {
		searchProvince(ProvinceCode, cacheMap)
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
			searchSub(ProvinceCode, ProvinceCode+sTypeCode, p.Cities)
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
				if changed {
					write(cacheMap)
				}

				return &Location{
					Province: p.Name,
					City:     city.Name,
					Distinct: distinct.Name,
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

func searchSub(pCode string, subCode string, parent map[string]*City) error {
	return search(pCode, subCode, func() Node {
		return new(City)
	}, func(code string, data interface{}) {
		if p, ok := data.(*City); ok {
			parent[code] = p
		}
	})
}

func searchDistinct(pCode string, subCode string, parent map[string]*DistinctMap) error {
	return search(pCode, subCode, func() Node {
		return new(DistinctMap)
	}, func(code string, data interface{}) {
		if p, ok := data.(*DistinctMap); ok {
			parent[code] = p
		}
	})
}

func searchStreet(pCode string, subCode string, parent map[string]*StreetMap) error {
	return search(pCode, subCode, func() Node {
		return new(StreetMap)
	}, func(code string, data interface{}) {
		if p, ok := data.(*StreetMap); ok {
			parent[code] = p
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
	} else if res.StatusCode >= 300 {
		err = errors.New(res.Status)
	}
	return
}
