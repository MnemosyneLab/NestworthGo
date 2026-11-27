package charts

import (
	"slices"
	"strings"
	"time"
)

// NoRow marks a Table without a selected row.
const NoRow = -1

// TableColumn is one column of a chart's data table.
type TableColumn struct {
	Title string
	// Numeric aligns the column to the trailing edge and keeps each cell on
	// one line, so amounts line up.
	Numeric bool
	// Wrap lets the column wrap when the table is narrower than its content.
	// Columns that neither wrap nor hold numbers, such as dates, keep their
	// width.
	Wrap bool
}

// Table is the plain-text form of a chart laid out as rows and columns, as the
// "View data table" disclosure under a Wails chart shows it. Cells carry the
// caller's exact text; the table never formats or rounds a value itself.
type Table struct {
	Columns []TableColumn
	Rows    [][]string
	// Selected is the index of the row of the selected chart item, or NoRow.
	Selected int
}

// TableLabels are the caller's column titles for the default tables.
type TableLabels struct {
	Date, Category, Amount, Share, Details string
}

// Table lays the series out by date: one row per date in time order, one
// column per series, and for a single series a details column joining each
// point's details when any point has them. A series without a point on a date
// leaves that cell empty.
func (s LineSpec) Table(labels TableLabels) Table {
	table := Table{Columns: []TableColumn{{Title: labels.Date}}, Selected: NoRow}
	var times []time.Time
	seen := make(map[int64]bool)
	for _, series := range s.Series {
		table.Columns = append(table.Columns, TableColumn{Title: series.Name, Numeric: true})
		for _, p := range series.Points {
			if key := p.Time.UnixNano(); !seen[key] {
				seen[key] = true
				times = append(times, p.Time)
			}
		}
	}
	slices.SortStableFunc(times, func(a, b time.Time) int { return a.Compare(b) })
	details := len(s.Series) == 1 && labels.Details != "" && slices.ContainsFunc(s.Series[0].Points, func(p LinePoint) bool { return len(p.Detail) > 0 })
	if details {
		table.Columns = append(table.Columns, TableColumn{Title: labels.Details, Wrap: true})
	}
	row := make(map[int64]int, len(times))
	for i, at := range times {
		row[at.UnixNano()] = i
		cells := make([]string, len(table.Columns))
		cells[0] = s.formatTime(at)
		table.Rows = append(table.Rows, cells)
	}
	for c, series := range s.Series {
		for _, p := range series.Points {
			i := row[p.Time.UnixNano()]
			value, _ := valueText(p.Value, p.Text, s.Messages)
			table.Rows[i][c+1] = value
			if details {
				table.Rows[i][len(table.Columns)-1] = strings.Join(p.Detail, " · ")
			}
			if s.Selected == (Selection{Series: series.ID, Item: p.ID}) {
				table.Selected = i
			}
		}
	}
	return table
}

// Table lists the slices in order with their amount and share. The share
// column appears when any slice has a share.
func (s DonutSpec) Table(labels TableLabels) Table {
	table := Table{Columns: []TableColumn{{Title: labels.Category, Wrap: true}, {Title: labels.Amount, Numeric: true}}, Selected: NoRow}
	shares := slices.ContainsFunc(s.Slices, func(sl Slice) bool { return sl.Share != "" })
	if shares {
		table.Columns = append(table.Columns, TableColumn{Title: labels.Share, Numeric: true})
	}
	for i, sl := range s.Slices {
		value, _ := valueText(sl.Value, sl.Text, s.Messages)
		cells := []string{sl.Label, value}
		if shares {
			cells = append(cells, sl.Share)
		}
		table.Rows = append(table.Rows, cells)
		if s.Selected == (Selection{Item: sl.ID}) {
			table.Selected = i
		}
	}
	return table
}

// Table lists every bar, including unavailable values.
func (s BarSpec) Table(labels TableLabels) Table {
	return itemTable(labels, s.Rows())
}

// Table lists every waterfall step, including totals and gaps.
func (s WaterfallSpec) Table(labels TableLabels) Table {
	return itemTable(labels, s.Rows())
}

// Table lists every node and link, including links omitted from the plot.
func (s SankeySpec) Table(labels TableLabels) Table {
	return itemTable(labels, s.Rows())
}

func itemTable(labels TableLabels, rows []TextRow) Table {
	table := Table{Columns: []TableColumn{{Title: labels.Category, Wrap: true}, {Title: labels.Amount, Numeric: true}}, Selected: NoRow}
	for i, row := range rows {
		label := row.Label
		if len(row.Detail) > 0 {
			label += "\n" + strings.Join(row.Detail, " · ")
		}
		table.Rows = append(table.Rows, []string{label, row.Value})
		if row.Selected {
			table.Selected = i
		}
	}
	return table
}
