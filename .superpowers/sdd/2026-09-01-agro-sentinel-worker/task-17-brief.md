### Task 17: Integration test — end-to-end processing

**Files:**
- Create: `tests/integration/processing_test.go`
- Create: `tests/testdata/` — small test GeoTIFFs

**Interfaces:**
- Consumes: everything built so far
- Produces: integration test that verifies the full pipeline works

- [ ] **Step 1: Create test GeoTIFFs**

Use GDAL to create small (10x10 pixel) test GeoTIFFs for B02, B03, B04, B08, SCL with known pixel values. These simulate a tiny Sentinel-2 scene.

- [ ] **Step 2: Write integration test**

Skip if GDAL not available. Test:
1. Create multiband.tif from test bands
2. Calculate cloud cover from test SCL
3. Generate natural.png
4. Generate NDVI image
5. Calculate statistics
6. Build params.json with mock previous scene
7. Verify all output files exist and have reasonable sizes

- [ ] **Step 3: Run and commit**

```bash
go test ./tests/integration/ -v -tags integration
git add .
git commit -m "test: end-to-end integration test for processing pipeline"
```
