//go:build windows

package fileassociation

import "testing"

func TestFileURL(t *testing.T) {
	for _, test := range []struct {
		path string
		want string
	}{
		{`D:\tmp\back-intercept-test.html`, `file:///D:/tmp/back-intercept-test.html`},
		{`D:\tmp\中文 目录\a#b.html`, `file:///D:/tmp/%E4%B8%AD%E6%96%87%20%E7%9B%AE%E5%BD%95/a%23b.html`},
		{`\\server\share\中文.html`, `file://server/share/%E4%B8%AD%E6%96%87.html`},
	} {
		if got := FileURL(test.path); got != test.want {
			t.Errorf("FileURL(%q) = %q, want %q", test.path, got, test.want)
		}
	}
}

func TestCommandPartsURLPlaceholder(t *testing.T) {
	path := `D:\tmp\中文 目录\a#b.html`
	parts, err := CommandParts(`msedge --app="%u"`, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 2 || parts[1] != `--app=file:///D:/tmp/%E4%B8%AD%E6%96%87%20%E7%9B%AE%E5%BD%95/a%23b.html` {
		t.Fatalf("URL 占位符替换结果不符: %#v", parts)
	}
	parts, err = CommandParts(`tool "%1" "%u"`, path)
	if err != nil || len(parts) != 3 || parts[1] != path || parts[2] != FileURL(path) {
		t.Fatalf("路径和 URL 占位符替换结果不符: %#v, %v", parts, err)
	}
}
