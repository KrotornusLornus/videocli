-- init.lua — punto de entrada del modo vídeos de swayimg.
--
-- Lanzar con:
--   swayimg --config ./swayimg-config-video/init.lua
--
-- Este archivo es solo configuración y carga de módulos; toda la lógica
-- vive en lib/ y la ejecuta el CLI videocli.

-- Settings generales ------------------------------------------------------

swayimg.mode = "gallery"          -- la galería es el modo principal
swayimg.title = "Videoteca"
swayimg.imagelist.fsmon = false   -- la lista la controla el CLI

-- Galería -----------------------------------------------------------------

swayimg.gallery.aspect = "keep"           -- respeta el aspecto de cada miniatura
swayimg.gallery.thumb_size = 240          -- tamaño de miniatura en píxeles
swayimg.gallery.padding_size = 8
swayimg.gallery.border_size = 2
swayimg.gallery.window_color = 0xff101014
swayimg.gallery.unselected_color = 0xff1c1c22
swayimg.gallery.selected_color = 0xff2a2a34
swayimg.gallery.border_color = 0xff4c8cff

-- Capa de texto -----------------------------------------------------------

swayimg.text.visible = true
swayimg.text.font = "monospace"
swayimg.text.size = 22
swayimg.text.padding = 12
swayimg.text.color = 0xffeeeeee
swayimg.text.background = 0x90101014
swayimg.text.status_timeout = 8
swayimg.text.timeout = 10

-- Esquema del texto en galería: ruta de la miniatura seleccionada.
swayimg.gallery.text = {
  bottomleft = {
    "{name}",
    "Índice:\t{list.index}/{list.total}"
  }
}

-- Carga de módulos --------------------------------------------------------

local here = debug.getinfo(1, "S").source:sub(2):match("^(.*/)[^/]+$") or "./"

VC = {
  dir = here,
  -- Carpeta que muestra la galería al arrancar (edítala aquí):
  start_dir = "~/Videos",
}

VC.util  = dofile(here .. "lib/util.lua")
VC.state = dofile(here .. "lib/state.lua")
VC.ui    = dofile(here .. "lib/ui.lua")
VC.cli   = dofile(here .. "lib/cli.lua")
dofile(here .. "lib/keys.lua")

-- Al arrancar: abre la galería en la carpeta inicial (VC.start_dir).
swayimg.on_initialized(function()
  VC.state.filter_dir = VC.util.expand(VC.start_dir)
  local n = VC.cli.refresh()
  VC.ui.status(("Videoteca: %s (%d vídeos)"):format(VC.state.filter_dir, n))
end)
