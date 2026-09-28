# Vehicle photos

Each file lives at `static/vehicles/<agency>/<mode>.jpg`, is 900px wide, and is shown in
the UI cropped to 16:9 with its credit printed under the image. To add an agency
or vehicle type, drop a photo in `static/vehicles/<agency>/` and register it in
`src/lib/vehicle.ts`.

| File | Vehicle | Author | License | Source |
| --- | --- | --- | --- | --- |
| `uta/bus.jpg` | UTA bus | Utah Transit | [CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0) | https://commons.wikimedia.org/wiki/File:Utah_Transit_Authority_route_72.jpg |
| `uta/light_rail.jpg` | TRAX light rail | GregMcCollum | [CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/) | https://commons.wikimedia.org/wiki/File:UTA_Trax_train_at_Stadium_Station,_October_15,_2023.jpg |
| `uta/commuter_rail.jpg` | FrontRunner | Jeffrey D. Allred / Deseret News | © Deseret News 2023, news photo used with the credit shown in the UI | N/A |
