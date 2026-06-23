import assert from 'node:assert/strict'

import {
  aggregateStageStatus,
  buildRunStageDisplays,
  isStageApproved,
  isStageConfirmed,
  mergeRunStageStatuses,
  mergeApprovedStageStatus,
  mergeTraceNodeStatuses,
  reviewStatusForStage,
} from '../src/pages/creatorWorkbenchLogic.ts'

assert.equal(aggregateStageStatus(['SUCCESS']), 'SUCCEEDED')

assert.deepEqual(
  buildRunStageDisplays(
    [{ name: 'recording_script', approvalRequired: false }],
    { recording_script: 'SUCCESS' },
    { recording_script: '口播稿' },
  ),
  [{ key: 'recording_script', label: '口播稿', status: 'SUCCEEDED', approvalStage: undefined }],
)

assert.equal(
  buildRunStageDisplays(
    [{ name: 'recording_script', approvalRequired: true }],
    { recording_script_exec: 'SUCCESS', recording_script: 'PENDING' },
    { recording_script: '口播稿' },
  )[0].status,
  'WAITING_APPROVAL',
)

assert.deepEqual(
  mergeApprovedStageStatus({ recording_script: 'WAITING_APPROVAL' }, 'recording_script'),
  { recording_script: 'SUCCEEDED', recording_script_exec: 'SUCCEEDED' },
)

assert.deepEqual(
  mergeTraceNodeStatuses(
    { recording_script: 'PENDING' },
    [{ id: 'recording_script', status: 'SUCCESS' }],
  ),
  { recording_script: 'SUCCEEDED' },
)

assert.deepEqual(
  mergeRunStageStatuses(
    { recording_script: 'SUCCEEDED', recording_script_exec: 'SUCCEEDED' },
    { recording_script: 'PENDING', recording_script_exec: 'SUCCESS' },
  ),
  { recording_script: 'SUCCEEDED', recording_script_exec: 'SUCCEEDED' },
)

assert.equal(isStageApproved({ recording_script: 'SUCCEEDED' }, 'recording_script', true), true)
assert.equal(isStageApproved({ recording_script: 'SUCCEEDED' }, 'recording_script', false), false)
assert.equal(isStageApproved({ recording_script: 'WAITING_APPROVAL' }, 'recording_script', true), false)

assert.equal(isStageConfirmed({ viewpoint_dossier: 'SUCCEEDED' }, 'viewpoint_dossier', false, true), true)
assert.equal(isStageConfirmed({ viewpoint_dossier: 'SUCCEEDED' }, 'viewpoint_dossier', false, false), false)
assert.equal(reviewStatusForStage({ recording_script_exec: 'SUCCEEDED', recording_script: 'PENDING' }, 'recording_script', true, false), 'PENDING_CONFIRMATION')
assert.equal(reviewStatusForStage({ recording_script: 'SUCCEEDED' }, 'recording_script', true, false), 'CONFIRMED')
assert.equal(reviewStatusForStage({ viewpoint_dossier: 'SUCCEEDED' }, 'viewpoint_dossier', false, true), 'CONFIRMED')

console.log('creatorWorkbenchLogic tests passed')
