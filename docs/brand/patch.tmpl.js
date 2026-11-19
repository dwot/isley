const byTitle = (t) => app.graph._nodes.find((n) => n.title === t);
byTitle('Prompt (Starter)').widgets[0].value = __PROMPT__;
const seed = byTitle('Int (Seed)'); seed.widgets[0].value = __SEED__; if (seed.widgets[1]) seed.widgets[1].value = 'fixed';
byTitle('Boolean (Enhance prompt)').widgets[0].value = false;
const loader = app.graph._nodes.find((n) => n.type === 'Lora Loader (LoraManager)');
const tw = loader.widgets.find((w) => w.name === 'text'); tw.value = ''; tw.callback?.(tw.value);
const rs = app.graph._nodes.find((n) => n.type === 'ResolutionSelector'); rs.widgets[0].value = __ASPECT__; rs.widgets[0].callback?.(rs.widgets[0].value);
