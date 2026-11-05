package domain

import "time"

type Menu struct {
	ID, ParentID                                           int64
	Name, Permission, Icon, Path, Component, ComponentName string
	Type, Sort, Status                                     int
	Visible, KeepAlive, AlwaysShow                         bool
	CreateTime                                             time.Time
}

func (m Menu) Validate() error {
	if !text(m.Name, 50) || m.ParentID < 0 || m.Type < 1 || m.Type > 3 || !ValidStatus(m.Status) || !optionalText(m.Permission, 100) || !optionalText(m.Icon, 100) || !optionalText(m.Path, 255) || !optionalText(m.Component, 255) || !optionalText(m.ComponentName, 100) {
		return ErrInvalidArgument
	}
	return nil
}
func ValidateMenuParent(menus []Menu, value Menu) error {
	depts := make([]Dept, 0, len(menus))
	for _, m := range menus {
		depts = append(depts, Dept{ID: m.ID, ParentID: m.ParentID})
		if m.ID == value.ParentID && m.Type == 3 {
			return ErrInvalidArgument
		}
	}
	if err := ValidateDeptParent(depts, value.ID, value.ParentID); err != nil {
		if err == ErrDeptNotFound {
			return ErrMenuNotFound
		}
		return err
	}
	return nil
}

type MenuNode struct {
	Menu
	Children []MenuNode
}

// EnabledMenus checks ancestors as well as the node, so disabling a directory
// also revokes the API permissions of its nested buttons.
func EnabledMenus(menus []Menu) []Menu {
	byID := make(map[int64]Menu, len(menus))
	for _, m := range menus {
		byID[m.ID] = m
	}
	result := []Menu{}
	for _, m := range menus {
		seen := map[int64]bool{}
		enabled := true
		for current := m.ID; current != 0; {
			node, ok := byID[current]
			if !ok || node.Status != StatusEnabled || seen[current] {
				enabled = false
				break
			}
			seen[current] = true
			current = node.ParentID
		}
		if enabled {
			result = append(result, m)
		}
	}
	return result
}

// MenuTree omits buttons and menus beneath disabled/missing parents.
func MenuTree(menus []Menu) []MenuNode {
	children := make(map[int64][]Menu)
	for _, m := range menus {
		if m.Status == StatusEnabled && m.Type != 3 {
			children[m.ParentID] = append(children[m.ParentID], m)
		}
	}
	var build func(int64, map[int64]bool) []MenuNode
	build = func(parent int64, seen map[int64]bool) []MenuNode {
		nodes := []MenuNode{}
		for _, m := range children[parent] {
			if seen[m.ID] {
				continue
			}
			seen[m.ID] = true
			nodes = append(nodes, MenuNode{Menu: m, Children: build(m.ID, seen)})
		}
		return nodes
	}
	return build(0, map[int64]bool{})
}
