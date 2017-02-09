package chinese_location_code

import (
	"fmt"
	"testing"
)

func TestSearch(t *testing.T) {
	fmt.Println(Search("510781"))
	fmt.Println(Search("110108"))
	fmt.Println(Search("110120"))
}
