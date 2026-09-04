### Task 9: RGB and band combination images

**Files:**
- Create: `internal/processing/rgb.go`
- Create: `internal/processing/indices.go`
- Test: `internal/processing/rgb_test.go`
- Test: `internal/processing/indices_test.go`

**Interfaces:**
- Consumes:
  - `gdal.Executor`, `gdal.Translate`, `gdal.BuildVRT`
  - `domain.Band`, `domain.FileType`
- Produces:
  - `processing.GenerateRGB(ctx, executor GDALExecutor, multibandPath string, outputPath string, redBand, greenBand, blueBand int) error`
    - Uses `gdal_translate -b R -b G -b B -of PNG -scale -ot Byte` to create 8-bit RGB PNG
  - `processing.GenerateIndex(ctx, executor GDALExecutor, multibandPath string, outputPath string, indexType domain.FileType) error`
    - Uses `gdal_calc.py` or VRT pixel functions to compute index, then `gdal_translate -of PNG` with color ramp
  - `processing.IndexDefinition` struct `{Type domain.FileType, Formula string, Bands []domain.Band, Name string}`
  - `processing.AllIndices() []IndexDefinition` — returns the 7 indices from the spec
  - `processing.AllCompositions() []CompositionDefinition` — returns natural, false_color, red_edge, swir
  - `processing.CompositionDefinition` struct `{Type domain.FileType, RedBand, GreenBand, BlueBand domain.Band, Name string}`

The band order in multiband.tif is: B02(1), B03(2), B04(3), B05(4), B06(5), B07(6), B08(7), B8A(8), B11(9), B12(10).

Compositions use band numbers from multiband.tif:
- natural.png: bands 3,2,1 (B04,B03,B02)
- false_color.png: bands 7,3,2 (B08,B04,B03)
- red_edge.png: bands 5,4,3 (B06,B05,B04)
- swir.png: bands 10,8,3 (B12,B8A,B04)

Index images use a color ramp (green-yellow-red for vegetation indices, blue-white-brown for moisture).

- [ ] **Step 1: Write composition and index definition tests**

Verify `AllIndices()` returns 7 entries, `AllCompositions()` returns 4 entries, band numbers map correctly.

- [ ] **Step 2: Write GenerateRGB test with mock executor**

Verify the gdal_translate command is built with correct `-b` flags and `-of PNG`.

- [ ] **Step 3: Implement rgb.go and indices.go**

- [ ] **Step 4: Run tests and commit**

```bash
go test ./internal/processing/ -v -run "RGB|Index"
git add .
git commit -m "feat: RGB compositions and vegetation index image generation"
```

---

