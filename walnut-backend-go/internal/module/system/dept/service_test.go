package dept

import (
	"testing"

	"walnut-seed-go/internal/pkg/modelx"
)

func deptRow(id, parent int64, name string, order int, status string) SysDept {
	p := parent
	n := name
	s := status
	o := order
	return SysDept{
		BaseModel: modelx.BaseModel{ID: id},
		ParentID:  &p,
		DeptName:  &n,
		OrderNum:  &o,
		Status:    &s,
		Ancestors: &s,
		DelFlag:   modelx.Normal,
	}
}

// TestBuildTreeSelect 覆盖下拉树的契约形态：键序由 struct 标签保证，
// 这里钉住的是**结构语义**——排序、children 键有无、多根、NULL 父级。
func TestBuildTreeSelect(t *testing.T) {
	t.Run("空列表返回空数组而非 null", func(t *testing.T) {
		got := BuildTreeSelect(nil)
		if got == nil || len(got) != 0 {
			t.Fatalf("want 空数组, got %#v", got)
		}
	})

	t.Run("排序与 children 键有无", func(t *testing.T) {
		rows := []SysDept{
			deptRow(100, 0, "根", 0, "0"),
			// 故意乱序传入：建树排序必须与输入顺序无关
			deptRow(102, 100, "二", 2, "0"),
			deptRow(101, 100, "一", 1, "0"),
			deptRow(103, 101, "叶", 1, "1"),
		}
		got := BuildTreeSelect(rows)
		if len(got) != 1 {
			t.Fatalf("want 1 个根, got %d", len(got))
		}
		root := got[0]
		if root.ID.Int64() != 100 || len(root.Children) != 2 {
			t.Fatalf("根形态错误: %#v", root)
		}
		if root.Children[0].ID.Int64() != 101 || root.Children[1].ID.Int64() != 102 {
			t.Fatalf("children 未按 orderNum 排序: %v, %v",
				root.Children[0].ID.Int64(), root.Children[1].ID.Int64())
		}
		leaf := root.Children[0].Children
		if len(leaf) != 1 || leaf[0].Children != nil {
			t.Fatalf("叶子不应带 children 键: %#v", leaf)
		}
		if !leaf[0].Disabled {
			t.Fatalf("status=1 应 disabled=true")
		}
	})

	t.Run("多根与 NULL 父级", func(t *testing.T) {
		orphan := deptRow(200, 999, "孤", 1, "0")
		orphan.ParentID = nil
		rows := []SysDept{
			deptRow(100, 0, "根", 0, "0"),
			deptRow(300, 100, "子", 5, "0"),
			orphan,
			// 父不在结果集内 → 也是根（过滤查询后的多根形态）
			deptRow(400, 888, "半孤", 2, "0"),
		}
		got := BuildTreeSelect(rows)
		if len(got) != 3 {
			t.Fatalf("want 3 个根(NULL/父缺失/正常根), got %d", len(got))
		}
		// 根按 (orderNum, id) 排序：根(0) < 孤(1) < 半孤(2)
		want := []int64{100, 200, 400}
		for i, w := range want {
			if got[i].ID.Int64() != w {
				t.Fatalf("根序[%d] want %d got %d", i, w, got[i].ID.Int64())
			}
		}
	})
}
