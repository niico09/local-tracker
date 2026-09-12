# Local Tracker — objetivos, series, películas y libros

## Origen

Con mi pareja nos pusimos como objetivo ver el Top 100 de series. Además, nos propusimos
objetivos anuales: leer libros, hacer cursos y otros objetivos personales.

## Motivación

Querer una herramienta propia, que se pueda customizar libremente. No busca reemplazar a
Trakt, Simkl, StoryGraph o Goodreads, sino tener control total sobre los datos y el
comportamiento.

## Qué es

Un programa self-hosted (se ejecuta en localhost y se accede por LAN) para hacer
seguimiento de objetivos y progreso de una pareja: qué serie o película se vio, qué libro
se leyó, qué cursos se hicieron, etc.

## Alcance de la v1

- Seguimiento binario: visto / no visto, leído / no leído, hecho / no hecho. Opcionalmente
  se registra fecha de inicio y fecha de fin.
- El Top 100 aplica únicamente a series.
- Películas, libros y cursos se cargan a mano. La búsqueda online para autocompletar
  portada y metadata queda como mejora futura.
- Se pueden registrar series vistas fuera del Top 100.
- Conviven objetivos de pareja (compartidos) y objetivos personales (privados).

## Modelo conceptual

- **Usuario**: cada persona. Identidad mínima (perfil + PIN).
- **Objetivo**: de pareja o personal. Tiene dueño, visibilidad y, opcionalmente, una meta
  numérica (por ejemplo, 100).
- **Ítem**: serie, película, libro o curso. Vive en un catálogo global y existe una sola
  vez. Puede tener id externo (las series del Top 100) o ser de carga manual. Guarda
  título, tipo, año y portada.
- **Pertenencia**: relación muchos a muchos entre ítem y objetivo. Un ítem puede estar en
  0, 1 o varios objetivos.
- **Progreso**: el tilde de un ítem. Guarda hecho, fecha de inicio, fecha de fin y dueño
  (la pareja o un usuario).

## Reglas de negocio

1. El dueño del progreso hereda el dueño del objetivo: objetivo de pareja → progreso
   compartido; objetivo personal → progreso individual y privado.
2. El Top 100 es un objetivo de pareja: se mide como un único avance compartido.
3. Los objetivos personales son privados por defecto; el dueño puede otorgar visibilidad
   al otro.
4. Un ítem suelto (fuera de todo objetivo) tiene progreso compartido por defecto, con
   opción de marcarlo como personal al cargarlo.
5. Las fechas son opcionales y permiten distinguir tres situaciones: pendiente (sin
   fechas), en curso (con inicio y sin fin) y completado (con ambas).

## Fuera de alcance (por ahora)

- Seguimiento por episodio.
- Multiusuario abierto, social y sincronización en la nube.
- Apps nativas.
- Integraciones automáticas con servicios externos.

## Stack

- **Lenguaje**: Go.
- **HTTP**: biblioteca estándar (`net/http`), sin framework.
- **UI**: HTML renderizado en el servidor con `html/template` + HTMX para interactividad.
  Tailwind para estilos.
- **Base de datos**: SQLite embebido, con driver puro Go (sin dependencias nativas).
- **Assets**: embebidos en el binario con `embed.FS` → un único ejecutable.
- **Cliente**: PWA para instalarlo y usarlo desde el celular.
- **Despliegue**: un solo binario que se ejecuta en la máquina host y escucha en la LAN.
