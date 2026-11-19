import { apiFetch } from './client'

export interface FlowSpaceIntegrationEvent {
  event_id: string
  kind: string
  status: string
  remote_status?: string
  error?: string
  created_at: string
}

export interface FlowSpaceIntegrationStatus {
  available: boolean
  connected: boolean
  connection_id?: string
  workspace_id?: string
  workspace_name?: string
  pending_count: number
  delivered_count: number
  last_error?: string
  events: FlowSpaceIntegrationEvent[]
}

const endpoint = '/api/v1/integrations/flowspace'

export function getFlowSpaceIntegration(signal?: AbortSignal) {
  return apiFetch<FlowSpaceIntegrationStatus>('GET', endpoint, undefined, signal)
}

export function pairFlowSpace(pairingCode: string, signal?: AbortSignal) {
  return apiFetch<FlowSpaceIntegrationStatus>('POST', `${endpoint}/pair`, { pairing_code: pairingCode }, signal)
}

export function disconnectFlowSpace(signal?: AbortSignal) {
  return apiFetch<FlowSpaceIntegrationStatus | undefined>('DELETE', endpoint, undefined, signal)
}

export function retryFlowSpaceIntegration(signal?: AbortSignal) {
  return apiFetch<FlowSpaceIntegrationStatus>('POST', `${endpoint}/retry`, undefined, signal)
}
