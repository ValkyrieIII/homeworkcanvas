<script setup lang="ts">
import { computed, onBeforeUnmount, ref, useId, watch } from "vue";

type Option = { value: string | number; label: string };
const props = defineProps<{
    modelValue: string | number;
    options: Option[];
    label: string;
    placeholder?: string;
}>();
const emit = defineEmits<{ "update:modelValue": [value: string | number] }>();

const uid = useId(),
    root = ref<HTMLElement | null>(null),
    trigger = ref<HTMLButtonElement | null>(null),
    open = ref(false),
    activeIndex = ref(-1);
const selectedIndex = computed(() =>
    props.options.findIndex((option) => option.value === props.modelValue),
);
const display = computed(
    () => props.options[selectedIndex.value]?.label ?? props.placeholder ?? "",
);
const optionId = (index: number) => `${uid}-option-${index}`;

function openList() {
    open.value = true;
    activeIndex.value = selectedIndex.value >= 0 ? selectedIndex.value : 0;
}
function closeList() {
    open.value = false;
    activeIndex.value = -1;
}
// 弹层不能落在 <label> 里：label 的激活行为会把内部点击转发给第一个可标记控件（也就是触发器），
// 于是选中一项时 closeList 刚置为 false，转发来的 click 立刻又把它打开。
function choose(index: number) {
    const option = props.options[index];
    if (!option) return;
    emit("update:modelValue", option.value);
    closeList();
    trigger.value?.focus();
}
function move(step: number) {
    const count = props.options.length;
    if (!count) return;
    const from = activeIndex.value < 0 ? (step > 0 ? -1 : 0) : activeIndex.value;
    activeIndex.value = (from + step + count) % count;
}
function onKeydown(event: KeyboardEvent) {
    if (!open.value) {
        if (["ArrowDown", "ArrowUp", "Enter", " "].includes(event.key)) {
            event.preventDefault();
            openList();
        }
        return;
    }
    const actions: Record<string, () => void> = {
        ArrowDown: () => move(1),
        ArrowUp: () => move(-1),
        Home: () => (activeIndex.value = 0),
        End: () => (activeIndex.value = props.options.length - 1),
        Enter: () => choose(activeIndex.value),
        " ": () => choose(activeIndex.value),
        Escape: closeList,
        Tab: closeList,
    };
    const action = actions[event.key];
    if (!action) return;
    event.preventDefault();
    action();
}
function onPointerDown(event: PointerEvent) {
    if (root.value && !root.value.contains(event.target as Node)) closeList();
}
watch(open, (value) => {
    if (value) document.addEventListener("pointerdown", onPointerDown);
    else document.removeEventListener("pointerdown", onPointerDown);
});
onBeforeUnmount(() => document.removeEventListener("pointerdown", onPointerDown));
</script>
<template>
    <div ref="root" class="select-menu">
        <button
            ref="trigger"
            type="button"
            class="select-menu__button"
            aria-haspopup="listbox"
            :aria-expanded="open"
            :aria-label="label"
            :aria-activedescendant="open && activeIndex >= 0 ? optionId(activeIndex) : undefined"
            @click="open ? closeList() : openList()"
            @keydown="onKeydown"
        >
            <span class="select-menu__value">{{ display }}</span>
            <span class="select-menu__caret" aria-hidden="true" />
            <span class="select-menu__sizer" aria-hidden="true">
                <span v-for="option in options" :key="option.value">{{ option.label }}</span>
            </span>
        </button>
        <ul v-if="open" class="select-menu__list" role="listbox" :aria-label="label">
            <li
                v-for="(option, index) in options"
                :id="optionId(index)"
                :key="option.value"
                class="select-menu__option"
                role="option"
                :aria-selected="option.value === modelValue"
                :class="{
                    'is-active': index === activeIndex,
                    'is-selected': option.value === modelValue,
                }"
                @click="choose(index)"
                @pointerenter="activeIndex = index"
            >
                {{ option.label }}
            </li>
        </ul>
    </div>
</template>
