import * as React from "react";
import * as ReactDOM from "react-dom";

import { Hello } from "./components/Hello";
import { Ticker } from "./components/Ticker";

ReactDOM.render(
  <>
    <Hello compiler="TypeScript" framework="React" />
    <Ticker />
  </>,
  document.getElementById("example")
);
