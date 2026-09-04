diff --git a/internal/processing/indices.go b/internal/processing/indices.go
new file mode 100644
index 0000000..0a42337
--- /dev/null
+++ b/internal/processing/indices.go
@@ -0,0 +1,209 @@
+package processing
+
+import (
+	"context"
+	"fmt"
+	"os"
+	"path/filepath"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+// IndexDefinition describes a vegetation/moisture index computed from
+// multiband.tif band numbers.
+type IndexDefinition struct {
+	Type    domain.FileType
+	Formula string
+	Bands   []domain.Band
+	Name    string
+}
+
+// moistureIndices identifies the indices that use the blue-white-brown
+// moisture color ramp instead of the green-yellow-red vegetation ramp.
+var moistureIndices = map[domain.FileType]bool{
+	domain.FileNBR:  true,
+	domain.FileNDMI: true,
+}
+
+// AllIndices returns the 7 vegetation/moisture indices defined by the spec.
+// Formulas reference gdal_calc.py letter placeholders A, B, C in the order
+// of the Bands slice.
+func AllIndices() []IndexDefinition {
+	return []IndexDefinition{
+		{
+			Type:    domain.FileNDVI,
+			Formula: "(A-B)/(A+B)",
+			Bands:   []domain.Band{domain.BandB08, domain.BandB04},
+			Name:    "NDVI",
+		},
+		{
+			Type:    domain.FileNDRE,
+			Formula: "(A-B)/(A+B)",
+			Bands:   []domain.Band{domain.BandB08, domain.BandB05},
+			Name:    "NDRE",
+		},
+		{
+			Type:    domain.FileEVI,
+			Formula: "2.5*(A-B)/(A+6*B-7.5*C+1)",
+			Bands:   []domain.Band{domain.BandB08, domain.BandB04, domain.BandB02},
+			Name:    "EVI",
+		},
+		{
+			Type:    domain.FileGNDVI,
+			Formula: "(A-B)/(A+B)",
+			Bands:   []domain.Band{domain.BandB08, domain.BandB03},
+			Name:    "GNDVI",
+		},
+		{
+			Type:    domain.FileNBR,
+			Formula: "(A-B)/(A+B)",
+			Bands:   []domain.Band{domain.BandB08, domain.BandB12},
+			Name:    "NBR",
+		},
+		{
+			Type:    domain.FileNDMI,
+			Formula: "(A-B)/(A+B)",
+			Bands:   []domain.Band{domain.BandB08, domain.BandB11},
+			Name:    "NDMI",
+		},
+		{
+			Type:    domain.FileSAVI,
+			Formula: "1.5*(A-B)/(A+B+0.5)",
+			Bands:   []domain.Band{domain.BandB08, domain.BandB04},
+			Name:    "SAVI",
+		},
+	}
+}
+
+// calcLetters are the gdal_calc.py input letters, in order, used to name
+// each band argument (-A, -B, -C, ...).
+var calcLetters = []string{"A", "B", "C", "D"}
+
+// vegetationColorRamp is a gdaldem color-relief ramp (green-yellow-red)
+// for vegetation indices, keyed on values in the [-1, 1] index range.
+const vegetationColorRamp = `-1.0 165 0 38
+-0.2 215 48 39
+0.0 254 224 139
+0.3 166 217 106
+0.6 26 152 80
+1.0 0 68 27
+`
+
+// moistureColorRamp is a gdaldem color-relief ramp (blue-white-brown) for
+// moisture-related indices, keyed on values in the [-1, 1] index range.
+const moistureColorRamp = `-1.0 140 81 10
+-0.2 223 194 125
+0.0 245 245 245
+0.2 128 205 193
+1.0 1 102 94
+`
+
+// GenerateIndex computes the vegetation/moisture index identified by
+// indexType from multibandPath and writes a color-mapped PNG to
+// outputPath. It uses gdal_calc.py to compute the raw index values into a
+// temporary GeoTIFF, then gdaldem color-relief to render the PNG.
+func GenerateIndex(ctx context.Context, executor GDALExecutor, multibandPath string, outputPath string, indexType domain.FileType) error {
+	def, ok := findIndexDefinition(indexType)
+	if !ok {
+		return &domain.ProcessingError{
+			Type:    domain.ErrValidation,
+			Message: "unknown index type: " + string(indexType),
+		}
+	}
+
+	outDir := filepath.Dir(outputPath)
+	rawPath := filepath.Join(outDir, string(indexType)+"_raw.tif")
+
+	if err := runIndexCalc(ctx, executor, def, multibandPath, rawPath); err != nil {
+		return err
+	}
+
+	colorFilePath := filepath.Join(outDir, string(indexType)+".clr")
+	ramp := vegetationColorRamp
+	if moistureIndices[indexType] {
+		ramp = moistureColorRamp
+	}
+	if err := os.WriteFile(colorFilePath, []byte(ramp), 0o644); err != nil {
+		return &domain.ProcessingError{
+			Type:    domain.ErrDisk,
+			Message: "writing color ramp file: " + colorFilePath,
+			Wrapped: err,
+		}
+	}
+
+	return runColorRelief(ctx, executor, rawPath, colorFilePath, outputPath)
+}
+
+func findIndexDefinition(indexType domain.FileType) (IndexDefinition, bool) {
+	for _, def := range AllIndices() {
+		if def.Type == indexType {
+			return def, true
+		}
+	}
+	return IndexDefinition{}, false
+}
+
+func runIndexCalc(ctx context.Context, executor GDALExecutor, def IndexDefinition, multibandPath, rawPath string) error {
+	args := []string{}
+	for i, band := range def.Bands {
+		letter := calcLetters[i]
+		args = append(args,
+			"-"+letter, multibandPath,
+			fmt.Sprintf("--%s_band=%d", letter, bandIndex[band]),
+		)
+	}
+	args = append(args,
+		"--calc="+def.Formula,
+		"--outfile="+rawPath,
+		"--type=Float32",
+	)
+
+	_, stderr, err := executor.Run(ctx, "gdal_calc.py", args)
+	if err != nil {
+		return &domain.ProcessingError{
+			Type:    domain.ErrGDAL,
+			Message: "gdal_calc.py failed computing " + def.Name,
+			Wrapped: err,
+		}
+	}
+
+	if _, statErr := os.Stat(rawPath); statErr != nil {
+		return &domain.ProcessingError{
+			Type:    domain.ErrGDAL,
+			Message: "gdal_calc.py did not produce output file: " + rawPath + " (" + stderr + ")",
+			Wrapped: statErr,
+		}
+	}
+
+	return nil
+}
+
+func runColorRelief(ctx context.Context, executor GDALExecutor, rawPath, colorFilePath, outputPath string) error {
+	args := []string{
+		"color-relief",
+		rawPath,
+		colorFilePath,
+		outputPath,
+		"-of", "PNG",
+		"-alpha",
+	}
+
+	_, stderr, err := executor.Run(ctx, "gdaldem", args)
+	if err != nil {
+		return &domain.ProcessingError{
+			Type:    domain.ErrGDAL,
+			Message: "gdaldem color-relief failed: " + outputPath,
+			Wrapped: err,
+		}
+	}
+
+	if _, statErr := os.Stat(outputPath); statErr != nil {
+		return &domain.ProcessingError{
+			Type:    domain.ErrGDAL,
+			Message: "gdaldem did not produce output file: " + outputPath + " (" + stderr + ")",
+			Wrapped: statErr,
+		}
+	}
+
+	return nil
+}
diff --git a/internal/processing/indices_test.go b/internal/processing/indices_test.go
new file mode 100644
index 0000000..a86ac5f
--- /dev/null
+++ b/internal/processing/indices_test.go
@@ -0,0 +1,166 @@
+package processing
+
+import (
+	"context"
+	"path/filepath"
+	"testing"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+func TestAllIndices(t *testing.T) {
+	indices := AllIndices()
+	if len(indices) != 7 {
+		t.Fatalf("expected 7 indices, got %d", len(indices))
+	}
+
+	wantBands := map[domain.FileType][]domain.Band{
+		domain.FileNDVI:  {domain.BandB08, domain.BandB04},
+		domain.FileNDRE:  {domain.BandB08, domain.BandB05},
+		domain.FileEVI:   {domain.BandB08, domain.BandB04, domain.BandB02},
+		domain.FileGNDVI: {domain.BandB08, domain.BandB03},
+		domain.FileNBR:   {domain.BandB08, domain.BandB12},
+		domain.FileNDMI:  {domain.BandB08, domain.BandB11},
+		domain.FileSAVI:  {domain.BandB08, domain.BandB04},
+	}
+
+	seen := map[domain.FileType]bool{}
+	for _, def := range indices {
+		seen[def.Type] = true
+		want, ok := wantBands[def.Type]
+		if !ok {
+			t.Fatalf("unexpected index type: %s", def.Type)
+		}
+		if len(def.Bands) != len(want) {
+			t.Fatalf("%s: bands = %v, want %v", def.Type, def.Bands, want)
+		}
+		for i, b := range want {
+			if def.Bands[i] != b {
+				t.Errorf("%s: bands[%d] = %s, want %s", def.Type, i, def.Bands[i], b)
+			}
+		}
+		if def.Formula == "" {
+			t.Errorf("%s: empty formula", def.Type)
+		}
+	}
+	for ft := range wantBands {
+		if !seen[ft] {
+			t.Errorf("missing index type: %s", ft)
+		}
+	}
+}
+
+func TestGenerateIndex_NDVI_BuildsCorrectCommands(t *testing.T) {
+	dir := t.TempDir()
+	multibandPath := filepath.Join(dir, "multiband.tif")
+	outputPath := filepath.Join(dir, "ndvi.png")
+
+	mock := &mockExecutor{}
+
+	if err := GenerateIndex(context.Background(), mock, multibandPath, outputPath, domain.FileNDVI); err != nil {
+		t.Fatalf("GenerateIndex() error = %v", err)
+	}
+
+	if len(mock.calls) != 2 {
+		t.Fatalf("expected 2 calls (gdal_calc.py, gdaldem), got %d: %+v", len(mock.calls), mock.calls)
+	}
+
+	calc := mock.calls[0]
+	if calc.command != "gdal_calc.py" {
+		t.Errorf("calls[0].command = %s, want gdal_calc.py", calc.command)
+	}
+	wantCalcArgs := []string{
+		"-A", multibandPath, "--A_band=7",
+		"-B", multibandPath, "--B_band=3",
+		"--calc=(A-B)/(A+B)",
+		"--outfile=" + filepath.Join(dir, "ndvi_raw.tif"),
+		"--type=Float32",
+	}
+	if len(calc.args) != len(wantCalcArgs) {
+		t.Fatalf("calc args = %v, want %v", calc.args, wantCalcArgs)
+	}
+	for i, a := range wantCalcArgs {
+		if calc.args[i] != a {
+			t.Errorf("calc args[%d] = %q, want %q", i, calc.args[i], a)
+		}
+	}
+
+	relief := mock.calls[1]
+	if relief.command != "gdaldem" {
+		t.Errorf("calls[1].command = %s, want gdaldem", relief.command)
+	}
+	if relief.args[0] != "color-relief" {
+		t.Errorf("relief args[0] = %s, want color-relief", relief.args[0])
+	}
+	if relief.args[3] != outputPath {
+		// args: color-relief raw color outputPath -of PNG -alpha
+		t.Errorf("relief args missing outputPath at expected position: %v", relief.args)
+	}
+}
+
+func TestGenerateIndex_EVI_UsesThreeBands(t *testing.T) {
+	dir := t.TempDir()
+	multibandPath := filepath.Join(dir, "multiband.tif")
+	outputPath := filepath.Join(dir, "evi.png")
+
+	mock := &mockExecutor{}
+
+	if err := GenerateIndex(context.Background(), mock, multibandPath, outputPath, domain.FileEVI); err != nil {
+		t.Fatalf("GenerateIndex() error = %v", err)
+	}
+
+	calc := mock.calls[0]
+	wantCalcArgs := []string{
+		"-A", multibandPath, "--A_band=7",
+		"-B", multibandPath, "--B_band=3",
+		"-C", multibandPath, "--C_band=1",
+		"--calc=2.5*(A-B)/(A+6*B-7.5*C+1)",
+		"--outfile=" + filepath.Join(dir, "evi_raw.tif"),
+		"--type=Float32",
+	}
+	if len(calc.args) != len(wantCalcArgs) {
+		t.Fatalf("calc args = %v, want %v", calc.args, wantCalcArgs)
+	}
+	for i, a := range wantCalcArgs {
+		if calc.args[i] != a {
+			t.Errorf("calc args[%d] = %q, want %q", i, calc.args[i], a)
+		}
+	}
+}
+
+func TestGenerateIndex_UnknownType(t *testing.T) {
+	dir := t.TempDir()
+	mock := &mockExecutor{}
+
+	err := GenerateIndex(context.Background(), mock, filepath.Join(dir, "multiband.tif"), filepath.Join(dir, "out.png"), domain.FileType("bogus"))
+	if err == nil {
+		t.Fatal("expected error for unknown index type")
+	}
+	pe, ok := err.(*domain.ProcessingError)
+	if !ok {
+		t.Fatalf("expected *domain.ProcessingError, got %T", err)
+	}
+	if pe.Type != domain.ErrValidation {
+		t.Errorf("Type = %s, want %s", pe.Type, domain.ErrValidation)
+	}
+	if len(mock.calls) != 0 {
+		t.Errorf("expected no executor calls, got %d", len(mock.calls))
+	}
+}
+
+func TestGenerateIndex_CalcExecutorError(t *testing.T) {
+	dir := t.TempDir()
+	mock := &mockExecutor{err: context.DeadlineExceeded}
+
+	err := GenerateIndex(context.Background(), mock, filepath.Join(dir, "multiband.tif"), filepath.Join(dir, "ndvi.png"), domain.FileNDVI)
+	if err == nil {
+		t.Fatal("expected error, got nil")
+	}
+	pe, ok := err.(*domain.ProcessingError)
+	if !ok {
+		t.Fatalf("expected *domain.ProcessingError, got %T", err)
+	}
+	if pe.Type != domain.ErrGDAL {
+		t.Errorf("Type = %s, want %s", pe.Type, domain.ErrGDAL)
+	}
+}
diff --git a/internal/processing/rgb.go b/internal/processing/rgb.go
new file mode 100644
index 0000000..5958d6c
--- /dev/null
+++ b/internal/processing/rgb.go
@@ -0,0 +1,79 @@
+package processing
+
+import (
+	"context"
+	"os"
+	"strconv"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+// CompositionDefinition describes a 3-band RGB composition built from
+// multiband.tif band numbers.
+type CompositionDefinition struct {
+	Type      domain.FileType
+	RedBand   domain.Band
+	GreenBand domain.Band
+	BlueBand  domain.Band
+	Name      string
+}
+
+// bandIndex maps a domain.Band to its 1-based band number in multiband.tif.
+// Band order: B02(1), B03(2), B04(3), B05(4), B06(5), B07(6), B08(7),
+// B8A(8), B11(9), B12(10).
+var bandIndex = map[domain.Band]int{
+	domain.BandB02: 1,
+	domain.BandB03: 2,
+	domain.BandB04: 3,
+	domain.BandB05: 4,
+	domain.BandB06: 5,
+	domain.BandB07: 6,
+	domain.BandB08: 7,
+	domain.BandB8A: 8,
+	domain.BandB11: 9,
+	domain.BandB12: 10,
+}
+
+// AllCompositions returns the standard RGB band compositions.
+func AllCompositions() []CompositionDefinition {
+	return []CompositionDefinition{
+		{Type: domain.FileNatural, RedBand: domain.BandB04, GreenBand: domain.BandB03, BlueBand: domain.BandB02, Name: "Natural Color"},
+		{Type: domain.FileFalseColor, RedBand: domain.BandB08, GreenBand: domain.BandB04, BlueBand: domain.BandB03, Name: "False Color"},
+		{Type: domain.FileRedEdge, RedBand: domain.BandB06, GreenBand: domain.BandB05, BlueBand: domain.BandB04, Name: "Red Edge"},
+		{Type: domain.FileSWIR, RedBand: domain.BandB12, GreenBand: domain.BandB8A, BlueBand: domain.BandB04, Name: "SWIR"},
+	}
+}
+
+// GenerateRGB creates an 8-bit RGB PNG from multibandPath using the given
+// 1-based band numbers for red, green, and blue.
+func GenerateRGB(ctx context.Context, executor GDALExecutor, multibandPath string, outputPath string, redBand, greenBand, blueBand int) error {
+	args := []string{
+		"-b", strconv.Itoa(redBand),
+		"-b", strconv.Itoa(greenBand),
+		"-b", strconv.Itoa(blueBand),
+		"-of", "PNG",
+		"-scale",
+		"-ot", "Byte",
+		multibandPath,
+		outputPath,
+	}
+
+	_, stderr, err := executor.Run(ctx, "gdal_translate", args)
+	if err != nil {
+		return &domain.ProcessingError{
+			Type:    domain.ErrGDAL,
+			Message: "gdal_translate failed generating RGB composition: " + outputPath,
+			Wrapped: err,
+		}
+	}
+
+	if _, statErr := os.Stat(outputPath); statErr != nil {
+		return &domain.ProcessingError{
+			Type:    domain.ErrGDAL,
+			Message: "gdal_translate did not produce output file: " + outputPath + " (" + stderr + ")",
+			Wrapped: statErr,
+		}
+	}
+
+	return nil
+}
diff --git a/internal/processing/rgb_test.go b/internal/processing/rgb_test.go
new file mode 100644
index 0000000..9f800e5
--- /dev/null
+++ b/internal/processing/rgb_test.go
@@ -0,0 +1,100 @@
+package processing
+
+import (
+	"context"
+	"path/filepath"
+	"testing"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+func TestAllCompositions(t *testing.T) {
+	comps := AllCompositions()
+	if len(comps) != 4 {
+		t.Fatalf("expected 4 compositions, got %d", len(comps))
+	}
+
+	want := map[domain.FileType][3]domain.Band{
+		domain.FileNatural:    {domain.BandB04, domain.BandB03, domain.BandB02},
+		domain.FileFalseColor: {domain.BandB08, domain.BandB04, domain.BandB03},
+		domain.FileRedEdge:    {domain.BandB06, domain.BandB05, domain.BandB04},
+		domain.FileSWIR:       {domain.BandB12, domain.BandB8A, domain.BandB04},
+	}
+
+	seen := map[domain.FileType]bool{}
+	for _, c := range comps {
+		seen[c.Type] = true
+		wantBands, ok := want[c.Type]
+		if !ok {
+			t.Fatalf("unexpected composition type: %s", c.Type)
+		}
+		if c.RedBand != wantBands[0] || c.GreenBand != wantBands[1] || c.BlueBand != wantBands[2] {
+			t.Errorf("%s: got RGB=%s,%s,%s want %s,%s,%s", c.Type, c.RedBand, c.GreenBand, c.BlueBand, wantBands[0], wantBands[1], wantBands[2])
+		}
+		if bandIndex[c.RedBand] == 0 || bandIndex[c.GreenBand] == 0 || bandIndex[c.BlueBand] == 0 {
+			t.Errorf("%s: band missing from bandIndex map", c.Type)
+		}
+	}
+	for ft := range want {
+		if !seen[ft] {
+			t.Errorf("missing composition type: %s", ft)
+		}
+	}
+}
+
+func TestGenerateRGB_BuildsCorrectCommand(t *testing.T) {
+	dir := t.TempDir()
+	multibandPath := filepath.Join(dir, "multiband.tif")
+	outputPath := filepath.Join(dir, "natural.png")
+
+	mock := &mockExecutor{}
+
+	if err := GenerateRGB(context.Background(), mock, multibandPath, outputPath, 3, 2, 1); err != nil {
+		t.Fatalf("GenerateRGB() error = %v", err)
+	}
+
+	if len(mock.calls) != 1 {
+		t.Fatalf("expected 1 call, got %d", len(mock.calls))
+	}
+
+	call := mock.calls[0]
+	if call.command != "gdal_translate" {
+		t.Errorf("command = %s, want gdal_translate", call.command)
+	}
+
+	wantArgs := []string{
+		"-b", "3", "-b", "2", "-b", "1",
+		"-of", "PNG",
+		"-scale",
+		"-ot", "Byte",
+		multibandPath,
+		outputPath,
+	}
+
+	if len(call.args) != len(wantArgs) {
+		t.Fatalf("args = %v, want %v", call.args, wantArgs)
+	}
+	for i, a := range wantArgs {
+		if call.args[i] != a {
+			t.Errorf("args[%d] = %q, want %q", i, call.args[i], a)
+		}
+	}
+}
+
+func TestGenerateRGB_ExecutorError(t *testing.T) {
+	dir := t.TempDir()
+	mock := &mockExecutor{err: context.DeadlineExceeded}
+
+	err := GenerateRGB(context.Background(), mock, filepath.Join(dir, "multiband.tif"), filepath.Join(dir, "natural.png"), 3, 2, 1)
+	if err == nil {
+		t.Fatal("expected error, got nil")
+	}
+
+	pe, ok := err.(*domain.ProcessingError)
+	if !ok {
+		t.Fatalf("expected *domain.ProcessingError, got %T", err)
+	}
+	if pe.Type != domain.ErrGDAL {
+		t.Errorf("Type = %s, want %s", pe.Type, domain.ErrGDAL)
+	}
+}
