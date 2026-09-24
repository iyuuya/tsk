package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestFuzzyMatches(t *testing.T) {
	rows := []string{
		"0\t.\tfoo\tfoo",
		"1\t.\tfzzoo\tfzzoo",
		"2\t.\tbar\tdeploy",
	}

	matches := fuzzyMatches(rows, "fo")
	if len(matches) != 2 {
		t.Fatalf("fuzzyMatches returned %d matches, want 2", len(matches))
	}
	if matches[0].index != 0 || matches[1].index != 1 {
		t.Errorf("fuzzyMatches order = %#v, want foo before format", matches)
	}

	matches = fuzzyMatches(rows, "dpy")
	if len(matches) != 1 || matches[0].index != 2 {
		t.Errorf("fuzzyMatches(dpy) = %#v, want deploy", matches)
	}
}

func TestPickerLabelAndTruncateDisplay(t *testing.T) {
	dir, adaptor, task, description := pickerRowFields("0\t.\tfake\tbuild\tcompile")
	if dir != "." || adaptor != "fake" || task != "build" || description != "compile" {
		t.Errorf("pickerRowFields() = %q, %q, %q, %q", dir, adaptor, task, description)
	}
	if got, want := truncateDisplay("あいうえお", 7), "あいう…"; got != want {
		t.Errorf("truncateDisplay() = %q, want %q", got, want)
	}
}

func TestFormatPickerFieldsAlignsColumns(t *testing.T) {
	columns := pickerColumnWidths{dir: 8, adaptor: 8, task: 8, description: 10}
	got := formatPickerFields(".", "mise", "build", "Build binary", columns)
	want := ".         mise      build     Build bin…"
	if got != want {
		t.Errorf("formatPickerFields() = %q, want %q", got, want)
	}

}

func TestFormatPickerFieldsOmitsDirectory(t *testing.T) {
	columns := pickerColumnWidths{adaptor: 8, task: 8, description: 10}
	got := formatPickerFields("DIR", "mise", "build", "Build binary", columns)
	want := "mise      build     Build bin…"
	if got != want {
		t.Errorf("formatPickerFields() = %q, want %q", got, want)
	}
}

func TestPickerColumnsOmitsUniqueDirectoryAndAdaptor(t *testing.T) {
	rows := []string{
		"0\t.\tmise\tbuild",
		"1\t.\tmise\ttest",
	}
	columns := pickerColumns(rows, 80)
	if columns.dir != 0 || columns.adaptor != 0 {
		t.Errorf("pickerColumns() = %+v, want unique directory and adaptor hidden", columns)
	}

	rows = append(rows, "2\tsub\tnpm\tbuild")
	columns = pickerColumns(rows, 80)
	if columns.dir == 0 || columns.adaptor == 0 {
		t.Errorf("pickerColumns() = %+v, want distinct directory and adaptor shown", columns)
	}
}

func TestRenderPickerUsesCRLF(t *testing.T) {
	var output bytes.Buffer
	rows := []string{"0\t.\tfake\tbuild\tcompile"}
	renderPicker(&output, "", rows, []pickerMatch{{index: 0}}, 0)

	if !strings.Contains(output.String(), "\r\n") {
		t.Errorf("renderPicker output = %q, want CRLF line endings", output.String())
	}
}
