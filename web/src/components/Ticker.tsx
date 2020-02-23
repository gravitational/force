import React, { useState, useEffect } from "react";

// Protobuf components
import { TickService } from '../proto/tick_pb_service';
import { TickRequest, Tick } from '../proto/tick_pb';

// GRPC
import { grpc } from "@improbable-eng/grpc-web";


// Ticker is a ticker function
export function Ticker() {
  // Declare a new state variable, which we'll call "count"
  const [count, setCount] = useState(0);

  const [tick, setTick] = useState("");

  // Similar to componentDidMount and componentDidUpdate:
  useEffect(() => {
    fetch("/api/ping")
      .then(response => response.json())
      .then(body => {
        setTick(body);
      });
    const tickRequest = new TickRequest();
    let request = grpc.invoke(TickService.Subscribe, {
      request: tickRequest,
      host: "https://localhost:8087",
      onMessage: (tick: Tick) => {
        setTick(new Date(tick.toObject().time / 1000000).toString())
        console.log("got tick: ", tick.toObject())
      },
      onEnd: (code: grpc.Code, msg: string | undefined, trailers: grpc.Metadata) => {
        if (code == grpc.Code.OK) {
          console.log("all ok")
        } else {
          console.log("hit an error", code, msg, trailers)
        }
      }
    });
    // stops subscription stream once component unmounts
    return () => { request.close() }
  }, [] /* tells React that it should not depend on grpc*/);

  return (
    <div>
      <p>You clicked {count} times, tick is {tick}</p>
      <button onClick={() => setCount(count + 1)}>
        Click me again
      </button>
    </div>
  );
}
