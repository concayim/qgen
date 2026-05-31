package gen

import "testing"

func TestParseBuckets(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		intent string
		want   int
	}{
		{
			"纯对象",
			`{"intelligent_qa":[{"question":"q1","answer_hint":"a"},{"question":"q2"}],"extract_uploaded":[{"question":"阅读上传的文件，提了哪几点意见"}]}`,
			"intelligent_qa", 2,
		},
		{
			"带代码块",
			"```json\n{\"write_from_doc\":[{\"question\":\"写一份传达稿\"}]}\n```",
			"write_from_doc", 1,
		},
		{
			"前后夹带文字",
			"好的：\n{\"rewrite_excerpt\":[{\"question\":\"把下面文字缩减为150字动态信息\"}]}\n完成",
			"rewrite_excerpt", 1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseBuckets(c.in)
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if len(got[c.intent]) != c.want {
				t.Fatalf("数量不符: got %d want %d", len(got[c.intent]), c.want)
			}
		})
	}
}

func TestParseBucketsInvalid(t *testing.T) {
	if _, err := parseBuckets("这里没有任何 JSON"); err == nil {
		t.Fatal("期望报错，但没有")
	}
}
