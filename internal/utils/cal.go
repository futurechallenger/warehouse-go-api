package utils

// =============================================================================
//  internal/utils/cal.go — 工具函数
//
//  提供与仓库平面图相关的几何/坐标计算辅助函数。
//  这部分是测试用的，可以自由修改。
// =============================================================================

import (
	"fmt"

	"warehouse/ent"
)

// Point 二维网格坐标
type Point struct {
	X, Y int
}

// Rect 矩形 (网格坐标)
type Rect struct {
	X, Y, W, H int
}

// CellSize 每格像素 (与前端/raylib 保持一致)
const CellSize = 58.0

// MarginX 画布左边距
const MarginX = 160.0

// MarginY 画布上边距
const MarginY = 120.0

// TitleBarH 标题栏高度
const TitleBarH = 52.0

// GridToPixel 网格坐标 → 像素中心
func GridToPixel(col, row int) (float64, float64) {
	x := MarginX + float64(col)*CellSize + CellSize/2
	y := TitleBarH + MarginY + float64(row)*CellSize + CellSize/2
	return x, y
}

// PixelToGrid 像素坐标 → 网格坐标 (取整)
func PixelToGrid(px, py float64) (int, int) {
	col := int((px - MarginX) / CellSize)
	row := int((py - TitleBarH - MarginY) / CellSize)
	return col, row
}

// Distance 曼哈顿距离 (网格单位)
func Distance(a, b Point) int {
	dx := a.X - b.X
	if dx < 0 {
		dx = -dx
	}
	dy := a.Y - b.Y
	if dy < 0 {
		dy = -dy
	}
	return dx + dy
}

// =============================================================================
//  货架布局查询 (与 service 层保持一致)
// =============================================================================

// ShelfLayout 货架布局参数
type ShelfLayout struct {
	Number       int
	Col, Row     int
	Width        int
	Height       int
	UnitCount    int
	IsHorizontal bool
}

// LayoutFromShelf 从 ent 货架实体构建布局参数（动态化，取代静态硬编码表）
func LayoutFromShelf(s *ent.Shelf) ShelfLayout {
	uc := len(s.Edges.Units)
	if uc == 0 {
		uc = 1
	}
	return ShelfLayout{
		Number:       s.ShelfNumber,
		Col:          s.Col,
		Row:          s.Row,
		Width:        s.Width,
		Height:       s.Height,
		UnitCount:    uc,
		IsHorizontal: s.IsHorizontal,
	}
}

// AccessPoint 根据布局计算某单元的存取通道格子坐标
func AccessPoint(l ShelfLayout, unitNum int) (col, row int, ok bool) {
	if unitNum < 1 || unitNum > l.UnitCount {
		return 0, 0, false
	}
	frac := UnitFrac(unitNum, l.UnitCount)
	row = l.Row + int(float64(l.Height)*frac)
	if l.IsHorizontal {
		// 横向货架: 存取点在下方
		row = l.Row + l.Height
		col = l.Col + l.Width/2
		return col, row, true
	}
	// 竖排货架: 存取点在右侧
	col = l.Col + l.Width
	return col, row, true
}

// GetShelfLayout 获取指定货架的布局参数（兼容静态演示表）
func GetShelfLayout(num int) (ShelfLayout, bool) {
	layouts := map[int]ShelfLayout{
		1:  {Number: 1, Col: 9, Row: 25, Width: 1, Height: 3, UnitCount: 4},
		2:  {Number: 2, Col: 9, Row: 21, Width: 1, Height: 3, UnitCount: 4},
		3:  {Number: 3, Col: 9, Row: 13, Width: 1, Height: 3, UnitCount: 4},
		4:  {Number: 4, Col: 6, Row: 13, Width: 1, Height: 3, UnitCount: 4},
		5:  {Number: 5, Col: 6, Row: 17, Width: 1, Height: 3, UnitCount: 3}, // 特殊: 3单元
		6:  {Number: 6, Col: 6, Row: 21, Width: 1, Height: 3, UnitCount: 4},
		7:  {Number: 7, Col: 3, Row: 21, Width: 1, Height: 3, UnitCount: 4},
		8:  {Number: 8, Col: 3, Row: 17, Width: 1, Height: 3, UnitCount: 4},
		9:  {Number: 9, Col: 3, Row: 13, Width: 1, Height: 3, UnitCount: 4},
		10: {Number: 10, Col: 3, Row: 0, Width: 6, Height: 1, UnitCount: 1, IsHorizontal: true}, // 横向×2
		11: {Number: 11, Col: 0, Row: 3, Width: 1, Height: 9, UnitCount: 6},                      // 竖排×3
	}
	s, ok := layouts[num]
	return s, ok
}

// GetAllShelves 获取所有货架布局
func GetAllShelves() []ShelfLayout {
	order := []int{10, 11, 9, 4, 3, 8, 5, 7, 6, 2, 1}
	var result []ShelfLayout
	for _, n := range order {
		if s, ok := GetShelfLayout(n); ok {
			result = append(result, s)
		}
	}
	return result
}

// =============================================================================
//  单元存取点计算
// =============================================================================

// CalcAccessCell 根据货架布局计算某单元的存取通道格子坐标
func CalcAccessCell(sCol, sRow, w, h, unit, uc int, isH bool) (col, row int) {
	frac := (float64(unit) - 0.5) / float64(uc)
	row = sRow + int(float64(h)*frac)
	if isH {
		row = sRow + h
		col = sCol + w/2
		return
	}
	col = sCol + w
	return
}

// UnitFrac 单元在货架内的竖向分数位置 (0~1)
func UnitFrac(unit, uc int) float64 {
	if uc <= 0 {
		return 0
	}
	return (float64(unit) - 0.5) / float64(uc)
}

// GetAccessPoint 获取某货架某单元的存取通道格子
func GetAccessPoint(shelfNum, unitNum int) (col, row int, ok bool) {
	s, ok := GetShelfLayout(shelfNum)
	if !ok || unitNum < 1 || unitNum > s.UnitCount {
		return 0, 0, false
	}

	frac := float64(unitNum) - 0.5/float64(s.UnitCount)
	row = s.Row + int(float64(s.Height)*frac)

	if s.Number == 10 {
		// 横向货架: 存取点在下方
		row = s.Row + s.Height
		col = s.Col + s.Width/2
		return col, row, true
	}

	// 竖排货架: 存取点在右侧
	col = s.Col + s.Width
	return col, row, true
}

// =============================================================================
//  绘图辅助
// =============================================================================

// GetShelfLabel 货架显示标签
func GetShelfLabel(num int) string {
	switch num {
	case 10:
		return "10 (×2)"
	case 11:
		return "11 (×3)"
	default:
		return fmt.Sprintf("%d", num)
	}
}

// IsBold 是否加深绘制
func IsBold(num int) bool {
	return num == 1 || num == 2
}

// UnitPositions 返回某货架所有单元的竖向分数位置 (0~1)
func UnitPositions(num int) []float64 {
	s, ok := GetShelfLayout(num)
	if !ok {
		return nil
	}
	pos := make([]float64, s.UnitCount)
	for i := 0; i < s.UnitCount; i++ {
		pos[i] = (float64(i) + 0.5) / float64(s.UnitCount)
	}
	return pos
}
