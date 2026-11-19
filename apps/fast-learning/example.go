package fastlearning

import "claude-code-go/pkg/appagent"

func (s *service) example() (Canvas, error) {
	overview := Node{
		ID: newID(), Question: "光刻机有哪些核心部件？", Method: "example",
		Sources: []appagent.Source{
			{Title: "ASML · 光刻原理", URL: "https://www.asml.com/en/technology/lithography-principles",
				Excerpt: "光刻系统利用光和精密光学，将掩模上的图形缩小并投影到感光晶圆上。"},
			{Title: "ASML · 运动与机电一体化", URL: "https://www.asml.com/en/technology/lithography-principles/mechanics-and-mechatronics",
				Excerpt: "工件台在曝光和测量位置之间移动，协调掩模和晶圆的精确运动。"},
		},
		Diagram: Diagram{
			Title:   "光刻机的核心构造",
			Summary: "把光刻机想成一台精密投影仪。光携带掩模上的图案，经投影光学系统缩小，在涂有光刻胶的晶圆上曝光。工件台不断移位和对准，以便在整片晶圆上重复印制。",
			Kind:    "flow",
			Concepts: []Concept{
				{Label: "光源与照明", Detail: "光源提供曝光所需波长，照明系统调节光束分布后将其送往掩模。", Question: "DUV 与 EUV 的光源有什么区别？", Refs: []int{1}},
				{Label: "掩模版", Detail: "掩模存有电路图案。经过掩模的光携带图案信息。", Question: "什么是掩模版，为什么不能直接接触晶圆？", Refs: []int{1}},
				{Label: "投影光学", Detail: "光学元件使图案缩小并成像。EUV 系统使用反射镜引导光束。", Question: "为什么 EUV 系统使用反射镜？", Refs: []int{1}},
				{Label: "晶圆与工件台", Detail: "涂有光刻胶的晶圆承接曝光，工件台把下一块待曝光区域送到指定位置。", Question: "晶圆台如何让不同位置依次曝光？", Refs: []int{1, 2}},
			},
			Edges: []Edge{{1, 2, "照亮"}, {2, 3, "传递图案"}, {3, 4, "缩小投影"}},
		},
	}
	branch := Node{
		ID: newID(), ParentID: overview.ID, Anchor: 4, Question: "晶圆台如何让不同位置依次曝光？", Method: "example",
		Sources: overview.Sources,
		Diagram: Diagram{
			Title:   "晶圆台上的定位与曝光",
			Summary: "晶圆上的重复图案来自多次局部曝光。设备先测量位置并对准，把目标区域移到成像位置，再与掩模运动配合完成曝光，之后继续处理下一块区域。",
			Kind:    "flow",
			Concepts: []Concept{
				{Label: "测量与对准", Detail: "测量系统建立晶圆和投影位置之间的关系，以便控制当前曝光的位置。", Question: "层与层之间为什么必须对准？", Refs: []int{2}},
				{Label: "协同扫描", Detail: "曝光时掩模与晶圆的运动要协调，让经过狭缝的图案投到正确位置。", Question: "扫描曝光和整场曝光有什么区别？", Refs: []int{2}},
				{Label: "移位重复", Detail: "一块区域曝光后，晶圆台移动到下一位置，同样的图案可以反复印制。", Question: "为什么晶圆边缘常有不完整的芯片？", Refs: []int{1}},
			},
			Edges: []Edge{{1, 2, "校准位置"}, {2, 3, "完成一场"}},
		},
	}
	return s.store.create(overview, branch)
}
