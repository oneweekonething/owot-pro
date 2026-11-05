package domain

import "time"

type Dept struct {
	ID, TenantID, ParentID, LeaderUserID int64
	Name, Phone, Email                   string
	Sort, Status                         int
	CreateTime                           time.Time
}

func (d Dept) Validate() error {
	if !text(d.Name, 30) || d.ParentID < 0 || d.LeaderUserID < 0 || !ValidStatus(d.Status) || !optionalText(d.Phone, 30) || !optionalText(d.Email, 100) {
		return ErrInvalidArgument
	}
	return nil
}

// DescendantIDs includes root and terminates even if persisted input is corrupt.
func DescendantIDs(depts []Dept, root int64) []int64 {
	children := make(map[int64][]int64)
	for _, d := range depts {
		children[d.ParentID] = append(children[d.ParentID], d.ID)
	}
	result := []int64{}
	seen := map[int64]bool{}
	queue := []int64{root}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
		queue = append(queue, children[id]...)
	}
	return result
}

func ValidateDeptParent(depts []Dept, id, parentID int64) error {
	if parentID == 0 {
		return nil
	}
	parents := map[int64]int64{}
	for _, d := range depts {
		parents[d.ID] = d.ParentID
	}
	seen := map[int64]bool{}
	for current := parentID; current != 0; {
		if current == id && id != 0 || seen[current] {
			return ErrTreeCycle
		}
		seen[current] = true
		next, ok := parents[current]
		if !ok {
			return ErrDeptNotFound
		}
		current = next
	}
	return nil
}
