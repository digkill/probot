import { useEffect, useRef, useState } from 'react'
import {
  ArcRotateCamera,
  Color3,
  Color4,
  Engine,
  HemisphericLight,
  MeshBuilder,
  Scene,
  StandardMaterial,
  Vector3,
  DynamicTexture,
} from '@babylonjs/core'
import { api, wsPath } from '../api'

type Campaign = { id: string; name: string }
type Graph = {
  nodes: { id: string; type: string; label: string; meta?: Record<string, unknown> }[]
  edges: { id: string; from: string; to: string; kind: string }[]
}

export default function GraphPage() {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const [campaigns, setCampaigns] = useState<Campaign[]>([])
  const [campaignId, setCampaignId] = useState('')
  const [info, setInfo] = useState('Pick a campaign')

  useEffect(() => {
    api<Campaign[]>(wsPath('/campaigns')).then((c) => {
      setCampaigns(c || [])
      if (c?.[0]) setCampaignId(c[0].id)
    }).catch((e) => setInfo(e.message))
  }, [])

  useEffect(() => {
    if (!campaignId || !canvasRef.current) return
    let disposed = false
    let engine: Engine | undefined

    ;(async () => {
      const graph = await api<Graph>(wsPath(`/campaigns/${campaignId}/graph`))
      if (disposed || !canvasRef.current) return
      engine = new Engine(canvasRef.current, true)
      const scene = new Scene(engine)
      scene.clearColor = new Color4(0.04, 0.07, 0.12, 1)
      const camera = new ArcRotateCamera('cam', Math.PI / 3, Math.PI / 3, 18, Vector3.Zero(), scene)
      camera.attachControl(canvasRef.current, true)
      new HemisphericLight('light', new Vector3(0, 1, 0), scene)

      const nodes = graph.nodes || []
      const edges = graph.edges || []
      const positions = new Map<string, Vector3>()
      nodes.forEach((n, i) => {
        const angle = (i / Math.max(nodes.length, 1)) * Math.PI * 2
        const radius = 6
        const pos = new Vector3(Math.cos(angle) * radius, (i % 3) - 1, Math.sin(angle) * radius)
        positions.set(n.id, pos)
        const sphere = MeshBuilder.CreateSphere(`n-${n.id}`, { diameter: n.type === 'url' ? 0.6 : 1.1 }, scene)
        sphere.position = pos
        const mat = new StandardMaterial(`m-${n.id}`, scene)
        mat.diffuseColor = n.type === 'url' ? Color3.FromHexString('#f0b429') : Color3.FromHexString('#3dd6c6')
        mat.emissiveColor = mat.diffuseColor.scale(0.25)
        sphere.material = mat

        const plane = MeshBuilder.CreatePlane(`t-${n.id}`, { width: 3, height: 0.6 }, scene)
        plane.position = pos.add(new Vector3(0, 1, 0))
        plane.billboardMode = 7
        const dt = new DynamicTexture(`dt-${n.id}`, { width: 512, height: 128 }, scene)
        dt.hasAlpha = true
        dt.drawText((n.label || n.id).slice(0, 40), null, 80, 'bold 28px IBM Plex Sans', '#e8eefc', 'transparent', true)
        const tm = new StandardMaterial(`tm-${n.id}`, scene)
        tm.diffuseTexture = dt
        tm.emissiveTexture = dt
        tm.opacityTexture = dt
        tm.backFaceCulling = false
        plane.material = tm
      })

      edges.forEach((e) => {
        const a = positions.get(e.from)
        const b = positions.get(e.to)
        if (!a || !b) return
        const lines = MeshBuilder.CreateLines(`e-${e.id}`, { points: [a, b] }, scene)
        lines.color = e.kind === 'canonical' ? Color3.FromHexString('#f0b429') : Color3.FromHexString('#8b9bb8')
      })

      setInfo(`${nodes.length} nodes · ${edges.length} edges`)
      engine.runRenderLoop(() => scene.render())
      const onResize = () => engine?.resize()
      window.addEventListener('resize', onResize)
      return () => window.removeEventListener('resize', onResize)
    })().catch((e) => setInfo(e.message))

    return () => {
      disposed = true
      engine?.dispose()
    }
  }, [campaignId])

  return (
    <div className="space-y-4">
      <div className="flex items-end justify-between gap-4">
        <div>
          <h2 className="text-2xl font-semibold">Amplification graph</h2>
          <p className="text-sm text-[var(--muted)]">{info}</p>
        </div>
        <select className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" value={campaignId} onChange={(e) => setCampaignId(e.target.value)}>
          {campaigns.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
        </select>
      </div>
      <canvas ref={canvasRef} className="w-full h-[70vh] rounded-xl border border-[var(--line)] bg-black/40" />
    </div>
  )
}
