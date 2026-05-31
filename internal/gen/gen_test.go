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
			`{"intelligent_qa":[{"question":"q1","answer_hint":"a"},{"question":"q2"}],"doc_extraction":[{"question":"列出所有责任部门"}]}`,
			"intelligent_qa", 2,
		},
		{
			"带代码块",
			"```json\n{\"info_writing\":[{\"question\":\"写讲话提纲\"}]}\n```",
			"info_writing", 1,
		},
		{
			"前后夹带文字",
			"好的：\n{\"draft_from_doc\":[{\"question\":\"把xxx改成动态信息\"}]}\n完成",
			"draft_from_doc", 1,
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
