import { describe, expect, it } from 'vitest'
import { getSignalFieldSchema } from '../lib/dslSchemas'
import {
  decisionQuestionTypes,
  signalAvailability,
  signalCapabilitySchema,
} from './signalCapabilities'
import type { DecisionTasks } from './useDecisionTasks'

function catalog(): DecisionTasks {
  const data: DecisionTasks = {
    default_deployment: 'primary',
    tasks: [],
    deployments: [],
    bindings: [],
    default_bindings: { domain: { deployment: 'primary' } },
  }
  for (const [id, type, binding, optional] of [
    ['domain', 'domain', 'domain', false],
    ['pii_presence', 'pii', 'pii', false],
    ['pii_categories', 'pii', 'pii', false],
    ['pii_spans', 'pii', 'pii', true],
    ['classifier', 'classifier', 'classifier.{name}', false],
  ] as const)
    data.tasks.push({
      id,
      title: id,
      description: '',
      stage: 'request',
      input: 'text',
      output: 'choice',
      full_input: true,
      template: { state: '', questions: {} },
      consumers: [{ kind: 'signal', type, binding, optional }],
    })
  data.deployments.push({
    deployment: 'primary',
    model: 'provider/judgment',
    ready: false,
    native_question_types: ['choice', 'noul'],
    tasks: data.tasks.map((task) => ({
      task_id: task.id,
      supported: ['pii_presence', 'pii_categories'].includes(task.id),
      reason: 'Unsupported native question',
      quality: 'unevaluated',
    })),
  })
  return data
}

describe('signal capability admission', () => {
  it('uses task capabilities, not readiness or model family', () => {
    const data = catalog()
    expect(signalAvailability(data, { recipe: 'default' }, 'domain').supported).toBe(false)
    expect(signalAvailability(data, { recipe: 'default' }, 'pii').supported).toBe(true)
    expect(signalAvailability(data, { recipe: 'default' }, 'keyword').supported).toBe(true)
  })
  it('keeps optional span detection from disabling a verdict task', () => {
    const data = catalog()
    expect(data.deployments[0].tasks.find((task) => task.task_id === 'pii_spans')?.supported).toBe(
      false,
    )
    expect(signalAvailability(data, { recipe: 'private' }, 'pii').supported).toBe(true)
    data.deployments[0].tasks.find((task) => task.task_id === 'pii_categories')!.supported = false
    expect(signalAvailability(data, { recipe: 'private' }, 'pii').supported).toBe(false)
  })
  it('honors explicit specialist overrides before default capabilities', () => {
    const data = catalog()
    data.global_bindings = { domain: { deployment: 'specialist' } }
    expect(signalAvailability(data, { recipe: 'private' }, 'domain').supported).toBe(true)
    expect(
      signalAvailability(data, { recipe: 'private', globalBindings: {} }, 'domain').supported,
    ).toBe(false)
    expect(
      signalAvailability(
        data,
        { recipe: 'private', bindings: { domain: { deployment: 'primary' } } },
        'domain',
      ).supported,
    ).toBe(false)
  })
  it('isolates a named recipe and does not borrow another recipe binding', () => {
    const data = catalog()
    data.bindings.push({
      task_id: 'domain',
      recipe: 'other',
      consumer: 'domain',
      source: 'recipe',
      deployment: 'specialist',
      model: '',
      ready: true,
      editable: true,
      path: [],
      binding: { deployment: 'specialist' },
    })
    expect(signalAvailability(data, { recipe: 'private' }, 'domain').supported).toBe(false)
  })
  it('allows composed Set through Noul but blocks unsupported Score and Span', () => {
    const data = catalog()
    const scope = { recipe: 'default' }
    expect(decisionQuestionTypes(data, scope, {})).toEqual(['choice', 'noul', 'set'])
    expect(
      signalAvailability(data, scope, 'systemone', 'needs', { question: { type: 'set' } }).supported,
    ).toBe(true)
    expect(
      signalAvailability(data, scope, 'systemone', 'difficulty', { question: { type: 'score' } })
        .supported,
    ).toBe(false)
    const schema = signalCapabilitySchema(
      getSignalFieldSchema('systemone'),
      decisionQuestionTypes(data, scope, {}),
    )
    const questionType = schema
      .find((item) => item.key === 'question')
      ?.fields?.find((item) => item.key === 'type')
    expect(questionType?.disabledOptions).toHaveProperty('score')
    expect(questionType?.disabledOptions).toHaveProperty('span')
    expect(questionType?.disabledOptions).not.toHaveProperty('set')
  })
  it('reads bindings written as deployment names under signal-type keys', () => {
    const data = catalog()
    data.global_bindings = { domain: 'specialist' }
    expect(signalAvailability(data, { recipe: 'private' }, 'domain').supported).toBe(true)
    expect(
      signalAvailability(data, { recipe: 'private', bindings: { domain: 'primary' } }, 'domain')
        .supported,
    ).toBe(false)
  })
  it('answers a rule from its deployment', () => {
    const data = catalog()
    // A rule-level type has no type-level binding.
    expect(
      signalAvailability(
        data,
        { recipe: 'private', bindings: { classifier: 'specialist' } },
        'classifier',
        'risk',
      ).supported,
    ).toBe(false)
    expect(
      signalAvailability(data, { recipe: 'private' }, 'classifier', 'risk', {
        deployment: 'specialist',
      }).supported,
    ).toBe(true)
    expect(
      signalAvailability(data, { recipe: 'private' }, 'classifier', 'risk', {
        deployment: 'primary',
      }).supported,
    ).toBe(false)
  })
  it('does not guess an unavailable model capability', () => {
    expect(signalAvailability(null, { recipe: 'default' }, 'domain').supported).toBe(false)
    expect(
      signalAvailability(catalog(), { recipe: 'default' }, 'systemone', '', {
        deployment: 'missing',
      }).supported,
    ).toBe(false)
  })
})
