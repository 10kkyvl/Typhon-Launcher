import { errorCode, msg } from '../i18n';
import { isEmulated } from '../library/emulation';
import type { LibraryGame } from '../services/library';
import {
  disableWrapper,
  disableWrapperKeepChanged,
  enableWrapper,
  inspectWrapper,
  onWrapperProgress,
  type WrapperId,
  type WrapperInfo,
  type WrapperProgress,
} from '../services/wrappers';
import type { GamePropertiesTarget } from '../stores/gameProperties';
import { toast } from '../stores/toasts';
import { focusableIn } from '../layers/focus';
import { focusHandOff } from '../utils/focusHandOff';
import { bytesSize } from '../utils/format';
import type { createFileWrites } from './fileWrites.svelte';
import { defaultWrapper, describeImports, presetsFor, problemActions, problemText, progressView } from './compatMode';
import { propertiesErrorText as errorText } from './propertiesErrors';

type Action = '' | 'revert' | 'keep';

/**
 * The compatibility mode (wrappers) switch of the game properties dialog: what Go says of the game's
 * wrapper, the switch and its problem actions. Created once by the dialog, which outlives its
 * openings (App mounts it once): `reset()` starts an opening, `load(gameId)` reads the game's state.
 */
export function createCompatSwitch(deps: {
  target: () => GamePropertiesTarget | null;
  game: () => LibraryGame | null;
  /** Taken at a press: whether the opening it was pressed in still hears its answer, and its game. */
  pressedNow: () => { heard: () => boolean; title: string };
  /** Why another write of the dialog holds the switch ('' when none does). */
  held: () => string;
  focus: { section: () => HTMLElement | undefined; props: () => HTMLElement | undefined };
  /** The dialog's read number: a read answered after a newer one began paints nothing. */
  seq: () => number;
  /** The dialog's file writes by game: the switch takes its game for as long as it runs. */
  writes: ReturnType<typeof createFileWrites>;
}) {
  let compat = $state<WrapperInfo | null>(null);
  let loading = $state(false);
  let error = $state('');
  let on = $state(false);
  let force = $state(false);
  let wrapper = $state<WrapperId>('dgvoodoo');
  let preset = $state('default');
  let progress = $state<WrapperProgress | null>(null);
  let keepOffer = $state(false);
  // Which problem action was pressed: that button stays focused and busy while the switch runs.
  let action = $state<Action>('');

  const busy = $derived(deps.writes.of(deps.game()?.id)?.kind === 'compat');
  const visible = $derived(Boolean(compat && (compat.supported || compat.active || compat.problem)));
  const suggested = $derived(compat?.suggested ?? '');
  const locked = $derived(!compat?.active && suggested === '' && !force);
  const api = $derived(compat ? describeImports(compat.imports) : '');
  const view = $derived(progress ? progressView(progress, bytesSize) : null);
  const actions = $derived(compat?.problem ? problemActions(compat.problem.code) : null);
  const files = $derived((compat?.active?.files ?? []).join(', '));

  $effect(() => {
    on = Boolean(compat?.active);
  });

  $effect(() => {
    const id = deps.game()?.id;
    if (!id) return;
    return onWrapperProgress((event) => {
      if (event.gameId === id) progress = event;
    });
  });

  async function load(gameId: string) {
    if (isEmulated(deps.game())) return;
    const mine = deps.seq();
    loading = true;
    try {
      const info = await inspectWrapper(gameId);
      if (mine !== deps.seq()) return;
      compat = info;
      wrapper = info.active ? info.active.wrapper : defaultWrapper(info);
      preset = info.active ? info.active.preset : 'default';
    } catch (err) {
      if (mine !== deps.seq()) return;
      error = errorText(err, msg('modals.compatInspectFailed'));
    } finally {
      if (mine === deps.seq()) loading = false;
    }
  }

  function pickWrapper(id: string) {
    wrapper = id as WrapperId;
    if (!presetsFor(wrapper).includes(preset)) preset = 'default';
  }

  async function change(turnOn: boolean, keepChanged = false, pressed: Action = '') {
    const game = deps.game();
    if (!game || busy || deps.held()) return;
    const id = game.id;
    const end = deps.writes.start(id, { kind: 'compat' });
    if (!end) return;
    const { heard, title } = deps.pressedNow();
    action = pressed;
    error = '';
    keepOffer = false;
    progress = null;
    // The dialog cannot be closed while this runs, yet it may be turned to another game or closed from
    // outside (the game removed). The switch stays known as running by game, across openings; its
    // outcome is said in the opening it was pressed in, else in a toast naming the game.
    const shown = () => deps.target()?.gameId === id;
    // A problem's action goes with its notice once it worked: focus it held goes to the compatibility
    // switch, which then says the outcome, else to the control that followed the notice.
    const hand = pressed ? focusHandOff() : null;
    const after = pressed ? controlAfterNotice() : null;
    let failure: string | undefined;
    try {
      if (turnOn) await enableWrapper(id, wrapper, preset);
      else if (keepChanged) await disableWrapperKeepChanged(id);
      else await disableWrapper(id);
    } catch (err) {
      failure = errorText(err, msg(turnOn ? 'modals.compatEnableFailed' : 'modals.compatDisableFailed'));
      if (heard()) {
        keepOffer = !turnOn && errorCode(err) === 'wrappers.tampered';
        error = failure;
      }
    } finally {
      end();
      if (heard()) action = '';
      if (shown()) progress = null;
    }
    if (!heard()) {
      if (failure === undefined) toast(msg('modals.propertiesLateSaved', { title }), 'success');
      else toast(msg('modals.compatLateFailed', { title, reason: (await recordedProblem(id)) ?? failure }), 'danger');
    }
    if (shown()) await load(id);
    if (heard()) await hand?.(() => deps.focus.section()?.querySelector<HTMLElement>('[role="switch"]') ?? (after?.isConnected ? after : null));
  }

  // A switch that failed and could not undo itself leaves a problem Go recorded (files partly
  // replaced): its words say more than the failure, which may carry no code at all.
  async function recordedProblem(id: string): Promise<string | null> {
    try {
      const problem = (await inspectWrapper(id)).problem;
      return problem ? problemText(problem.code, problem.file) : null;
    } catch (err) {
      console.warn('read the wrapper problem after a late failure', err);
      return null;
    }
  }

  function controlAfterNotice(): HTMLElement | null {
    const notice = document.activeElement?.closest('.notice');
    const props = deps.focus.props();
    if (!notice || !props) return null;
    const all = focusableIn(props);
    const mine = all.filter((el) => notice.contains(el));
    return all[all.indexOf(mine[mine.length - 1]) + 1] ?? null;
  }

  return {
    get compat() {
      return compat;
    },
    get loading() {
      return loading;
    },
    get busy() {
      return busy;
    },
    get error() {
      return error;
    },
    get on() {
      return on;
    },
    get force() {
      return force;
    },
    get wrapper() {
      return wrapper;
    },
    get preset() {
      return preset;
    },
    set preset(value: string) {
      preset = value;
    },
    get keepOffer() {
      return keepOffer;
    },
    get action() {
      return action;
    },
    get visible() {
      return visible;
    },
    get suggested() {
      return suggested;
    },
    get locked() {
      return locked;
    },
    get api() {
      return api;
    },
    get view() {
      return view;
    },
    get actions() {
      return actions;
    },
    get files() {
      return files;
    },

    /** A new opening: nothing of the last one's game is shown. */
    reset() {
      action = '';
      compat = null;
      error = '';
      force = false;
      progress = null;
      keepOffer = false;
    },
    load,
    pickWrapper,
    toggleForce() {
      force = !force;
    },
    /** The switch pressed: it shows the change while the change runs; a held one ignores the press. */
    toggle(turnOn: boolean) {
      if (busy || deps.held()) return;
      on = turnOn;
      void change(turnOn);
    },
    revert: () => change(false, false, 'revert'),
    keep: () => change(false, true, 'keep'),
  };
}
