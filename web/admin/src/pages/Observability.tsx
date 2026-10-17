import { useCallback, useEffect, useState } from 'react'
import { ObservabilityPanel, type ObservationHistoryPoint } from './Stun'
import { Card, CardHeader } from '../components/Card'
import * as api from '../lib/api'
import type { ObservabilitySnapshot, SystemObservation } from '../types'

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0.00 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let n = value
  let unit = 0
  while (n >= 1024 && unit < units.length - 1) {
    n /= 1024
    unit++
  }
  return `${n.toFixed(2)} ${units[unit]}`
}

function displayBytes(value: number) {
  return value > 0 ? formatBytes(value) : '--'
}

function formatPercent(used: number, total: number) {
  if (!total) return '--'
  return `${((used / total) * 100).toFixed(2)}%`
}

function formatUptime(seconds: number) {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days}天 ${hours}小时`
  if (hours > 0) return `${hours}小时 ${minutes}分钟`
  return `${minutes}分钟`
}

function SystemResourcePanel({
  data,
  error,
}: {
  data: SystemObservation | null
  error: string
}) {
  const memory = data?.memory
  const disk = data?.disk
  const process = data?.process

  return (
    <Card>
      <CardHeader
        title="系统资源"
        action={
          <span className="text-xs text-slate-400">
            {data?.sampledAt ? `采样于 ${new Date(data.sampledAt).toLocaleString()}` : '等待采样'}
          </span>
        }
      />
      {error && (
        <div className="mb-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700">
          系统资源暂时不可用：{error}
        </div>
      )}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <div className="rounded-xl bg-blue-50 p-3 ring-1 ring-blue-100">
          <div className="text-xs text-blue-600">进程 CPU</div>
          <div className="mt-1 text-lg font-bold text-slate-800">{process ? `${process.cpuPercent.toFixed(2)}%` : '--'}</div>
          <div className="mt-1 text-[11px] text-slate-400">{data ? `${data.cpuCount} 核 CPU` : '--'}</div>
        </div>
        <div className="rounded-xl bg-violet-50 p-3 ring-1 ring-violet-100">
          <div className="text-xs text-violet-600">主机内存</div>
          <div className="mt-1 text-lg font-bold text-slate-800">{memory ? formatPercent(memory.usedBytes, memory.totalBytes) : '--'}</div>
          <div className="mt-1 text-[11px] text-slate-400">
            {memory ? `${displayBytes(memory.usedBytes)} / ${displayBytes(memory.totalBytes)}` : '--'}
          </div>
        </div>
        <div className="rounded-xl bg-amber-50 p-3 ring-1 ring-amber-100">
          <div className="text-xs text-amber-600">工作目录磁盘</div>
          <div className="mt-1 text-lg font-bold text-slate-800">{disk ? formatPercent(disk.usedBytes, disk.totalBytes) : '--'}</div>
          <div className="mt-1 text-[11px] text-slate-400">剩余 {disk ? displayBytes(disk.freeBytes) : '--'}</div>
        </div>
        <div className="rounded-xl bg-emerald-50 p-3 ring-1 ring-emerald-100">
          <div className="text-xs text-emerald-600">进程状态</div>
          <div className="mt-1 text-lg font-bold text-slate-800">{process ? formatUptime(process.uptimeSeconds) : '--'}</div>
          <div className="mt-1 text-[11px] text-slate-400">{process ? `${process.goroutines} 个协程` : '--'}</div>
        </div>
      </div>
      {data && (
        <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-[11px] text-slate-500">
          <span>系统：<b className="font-medium text-slate-700">{data.os} / {data.arch}</b></span>
          <span>进程内存：<b className="font-medium text-slate-700">{displayBytes(data.process.memoryBytes)}</b></span>
          <span>堆内存：<b className="font-medium text-slate-700">{displayBytes(data.process.heapBytes)}</b></span>
          <span>磁盘：<b className="font-medium text-slate-700">{data.disk.path}</b></span>
          {data.disk.error && <span className="text-rose-500">{data.disk.error}</span>}
        </div>
      )}
    </Card>
  )
}

export function Observability() {
  const [data, setData] = useState<ObservabilitySnapshot | null>(null)
  const [history, setHistory] = useState<ObservationHistoryPoint[]>([])
  const [networkError, setNetworkError] = useState('')
  const [system, setSystem] = useState<SystemObservation | null>(null)
  const [systemError, setSystemError] = useState('')

  const refresh = useCallback(async () => {
    const [networkResult, systemResult] = await Promise.allSettled([
      api.getStunObservability(),
      api.getSystemObservability(),
    ])

    if (networkResult.status === 'fulfilled') {
      const snapshot = networkResult.value
      setData(snapshot)
      setNetworkError('')
      setHistory((previous) => [
        ...previous,
        {
          sampledAt: snapshot.sampledAt,
          inBytesPerSecond: snapshot.traffic.inBytesPerSecond,
          outBytesPerSecond: snapshot.traffic.outBytesPerSecond,
        },
      ].slice(-30))
    } else {
      setNetworkError(networkResult.reason instanceof Error ? networkResult.reason.message : String(networkResult.reason))
    }

    if (systemResult.status === 'fulfilled') {
      setSystem(systemResult.value)
      setSystemError('')
    } else {
      setSystemError(systemResult.reason instanceof Error ? systemResult.reason.message : String(systemResult.reason))
    }
  }, [])

  useEffect(() => {
    const first = window.setTimeout(() => {
      void refresh()
    }, 0)
    const timer = window.setInterval(refresh, 2000)
    return () => {
      window.clearTimeout(first)
      window.clearInterval(timer)
    }
  }, [refresh])

  return (
    <div className="space-y-4">
      <SystemResourcePanel data={system} error={systemError} />
      <ObservabilityPanel data={data} history={history} error={networkError} onRefresh={refresh} />
    </div>
  )
}
