/// <reference types="vite/client" />

declare module 'georaster' {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const parseGeoraster: (input: ArrayBuffer | string) => Promise<any>
  export default parseGeoraster
}

declare module 'georaster-layer-for-leaflet' {
  import type { GridLayer, GridLayerOptions } from 'leaflet'
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  class GeoRasterLayer extends GridLayer {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    constructor(options: GridLayerOptions & { georaster: any; opacity?: number; resolution?: number; pixelValuesToColorFn?: (values: number[]) => string })
    getBounds(): import('leaflet').LatLngBounds
  }
  export default GeoRasterLayer
}
