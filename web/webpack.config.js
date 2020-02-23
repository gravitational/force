const path = require('path');
const HtmlWebpackPlugin = require('html-webpack-plugin');
const CopyPlugin = require('copy-webpack-plugin');

module.exports = {
  mode: 'development',

  // Enable sourcemaps for debugging webpack's output.
  // https://webpack.js.org/guides/development/
  devtool: 'inline-source-map',
  devServer: {
  },
  entry: './src/index.tsx',

  resolve: {
    // Add '.ts' and '.tsx' as resolvable extensions.
    extensions: [".ts", ".tsx", ".js", ".jsx"],
  },

  module: {
    rules: [
      {
        test: /\.ts(x?)$/,
        exclude: /node_modules/,
        use: [
          {
            loader: "ts-loader"
          }
        ]
      },
      // All output '.js' files will have any sourcemaps re-processed by 'source-map-loader'.
      {
        enforce: "pre",
        test: /\.js$/,
        loader: "source-map-loader"
      },
      // This fix is required to fix google protobuf code gen
      // that generates this line:
      //
      // var global = Function('return this')();
      // 
      // violating CSP policy script-src 'self'.
      // Thankfully, it's trivial to fix by using this code instead:
      //
      // var global = (function(){ return this }).call(null);
      // 
      //
      // Read more here:
      //
      // https://github.com/improbable-eng/ts-protoc-gen/issues/128
      // https://github.com/protocolbuffers/protobuf/issues/6770
      {
        test: /\.js$/,
        loader: 'string-replace-loader',
        options: {
          search: "var global = Function('return this')();",
          replace: "var global = (function(){ return this }).call(null);",
        }
      },
    ]
  },
  plugins: [
    new HtmlWebpackPlugin({
      template: './index.html'
    }),
    new CopyPlugin([
      { from: './node_modules/react/umd/react.development.js', to: path.resolve(__dirname, 'dist') },
      { from: './node_modules/react-dom/umd/react-dom.development.js', to: path.resolve(__dirname, 'dist') },
    ]),
  ], 

  // When importing a module whose path matches one of the following, just
  // assume a corresponding global variable exists and use that instead.
  // This is important because it allows us to avoid bundling all of our
  // dependencies, which allows browsers to cache those libraries between builds.
  externals: {
    "react": "React",
    "react-dom": "ReactDOM"
  },

  output: {
    filename: '[name].bundle.js',
    path: path.resolve(__dirname, 'dist'),
    publicPath: "/web/dist",
  },
};
