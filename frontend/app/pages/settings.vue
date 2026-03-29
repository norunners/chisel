<script setup lang="ts">
import * as models from '../../bindings/github.com/norunners/chisel/models'

const toast = useChiselToast()
const {
  settings,
  agents,
  loading,
  testResults,
  refreshSettings,
  saveSettings,
  testAgent
} = useChiselSettings()

const editorValue = ref('')
const saving = ref(false)
const testing = ref<Record<string, boolean>>({})

function resultFor(agentName: string) {
  return testResults.value[agentName]
}

onMounted(async () => {
  await refreshSettings()
})

watch(settings, (value) => {
  editorValue.value = JSON.stringify(value, null, 2)
}, { immediate: true, deep: true })

async function handleSave() {
  saving.value = true
  try {
    const parsed = models.ChiselSettings.createFrom(JSON.parse(editorValue.value))
    await saveSettings(parsed)
    toast.add({
      title: 'Settings saved',
      description: 'Chisel updated `~/.chisel/settings.json`.',
      icon: 'i-lucide-save'
    })
  } catch (error) {
    toast.add({
      title: 'Unable to save settings',
      description: error instanceof Error ? error.message : 'The JSON is invalid or could not be written.',
      color: 'error'
    })
  } finally {
    saving.value = false
  }
}

async function handleTest(agentName: string) {
  testing.value = {
    ...testing.value,
    [agentName]: true
  }

  try {
    await testAgent(agentName)
    toast.add({
      title: 'Agent initialized',
      description: `${agentName} completed ACP initialize successfully.`,
      icon: 'i-lucide-badge-check'
    })
  } catch (error) {
    toast.add({
      title: 'Agent test failed',
      description: error instanceof Error ? error.message : 'Unable to initialize the ACP backend.',
      color: 'error'
    })
  } finally {
    testing.value = {
      ...testing.value,
      [agentName]: false
    }
  }
}
</script>

<template>
  <UDashboardPanel
    id="settings"
    class="min-h-0"
    :ui="{ body: 'p-0 sm:p-0' }"
  >
    <template #header>
      <DashboardNavbar />
    </template>

    <template #body>
      <UContainer class="flex-1 py-8 space-y-6">
        <div class="space-y-2">
          <p class="text-sm uppercase tracking-[0.2em] text-muted">
            Settings
          </p>
          <h1 class="text-3xl font-bold text-highlighted">
            Manage ACP agents and local session state.
          </h1>
          <p class="text-sm text-muted max-w-3xl">
            The JSON editor below writes directly to `~/.chisel/settings.json`. Chisel keeps only a lightweight local session index; transcripts stay with the ACP backend.
          </p>
        </div>

        <div class="grid gap-6 xl:grid-cols-[1.2fr_0.8fr]">
          <section class="rounded-2xl border border-default bg-default/80 p-4 sm:p-5 shadow-sm space-y-4">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div>
                <h2 class="text-lg font-semibold text-highlighted">
                  Agent Registry
                </h2>
                <p class="text-sm text-muted">
                  Edit the exact schema Chisel reads at startup.
                </p>
              </div>

              <div class="flex gap-2">
                <UButton
                  label="Reload"
                  color="neutral"
                  variant="outline"
                  :loading="loading"
                  @click="refreshSettings"
                />
                <UButton
                  label="Save"
                  icon="i-lucide-save"
                  color="neutral"
                  :loading="saving"
                  @click="handleSave"
                />
              </div>
            </div>

            <textarea
              v-model="editorValue"
              class="min-h-[28rem] w-full rounded-xl border border-default bg-default px-4 py-3 font-mono text-sm"
              spellcheck="false"
            />
          </section>

          <div class="space-y-6">
            <section class="rounded-2xl border border-default bg-default/80 p-4 sm:p-5 shadow-sm space-y-4">
              <div class="space-y-1">
                <h2 class="text-lg font-semibold text-highlighted">
                  Launch Tests
                </h2>
                <p class="text-sm text-muted">
                  Runs ACP `initialize` only. It does not create or load a session.
                </p>
              </div>

              <div class="space-y-3">
                <div
                  v-for="agent in agents"
                  :key="agent.name"
                  class="rounded-xl border border-default bg-default/60 p-3 space-y-2"
                >
                  <div class="flex items-start justify-between gap-3">
                    <div>
                      <p class="text-sm font-medium text-highlighted">
                        {{ agent.name }}
                      </p>
                      <p class="text-xs text-muted break-all">
                        {{ agent.command }} {{ agent.args.join(' ') }}
                      </p>
                    </div>

                    <UButton
                      label="Test"
                      color="neutral"
                      variant="soft"
                      :loading="testing[agent.name]"
                      @click="handleTest(agent.name)"
                    />
                  </div>

                  <div v-if="resultFor(agent.name)" class="rounded-lg bg-elevated/60 p-3 text-sm space-y-1">
                    <p class="font-medium text-highlighted">
                      {{ resultFor(agent.name)?.agentTitle || agent.name }}
                    </p>
                    <p class="text-muted">
                      ACP v{{ resultFor(agent.name)?.protocolVersion }} · {{ resultFor(agent.name)?.agentVersion || 'unknown version' }}
                    </p>
                    <p class="text-muted">
                      Load session: {{ resultFor(agent.name)?.loadSession ? 'yes' : 'no' }}
                    </p>
                    <p class="text-muted">
                      Prompt types: {{ resultFor(agent.name)?.promptCapabilities.image ? 'text + image' : 'text only' }}
                    </p>
                    <p
                      v-for="method in resultFor(agent.name)?.authMethods || []"
                      :key="method.id"
                      class="text-xs text-muted"
                    >
                      {{ method.name }}: {{ method.description }}
                    </p>
                  </div>
                </div>
              </div>
            </section>
          </div>
        </div>
      </UContainer>
    </template>
  </UDashboardPanel>
</template>
